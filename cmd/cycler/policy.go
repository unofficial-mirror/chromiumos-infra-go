// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"go.chromium.org/chromiumos/infra/go/cmd/cycler/effects"

	"cloud.google.com/go/storage"
	"github.com/golang/glog"
	"github.com/open-policy-agent/opa/rego"
)

// Policy defines a policy document, settings, and runtime information.
type Policy struct {
	// The policy effect configuration (contains effect_configuration).
	Config cycler_pb.PolicyEffectConfiguration `json:"PolicyEffectConfiguration"`

	// We must be explicitly allowed to mutate after policy determinations.
	MutationAllowed bool `json:"MutationAllowed"`

	// Because it's low in runtime cost, and we're touching the objects
	// regardless, also update an instance of Stats.
	PrefixStats *Stats `json:"PrefixStats"`

	// Make stats for all the objects we act on as well ('as' -> actionStats).
	ActionStats *Stats `json:"ActionStats"`

	// The effect we've configured.
	Effect effects.Effect `json:"Effect"` // effect.effectEffect(effect, effect, ...)    ;)

	// The rego context object to build.
	r *rego.Rego

	// The prepared (via init()) query to run on the submitted objects.
	q *rego.PreparedEvalQuery

	// The compiled regex to apply to each prefix.
	prefixRegexp *regexp.Regexp

	// gcp client, set on init.
	client *storage.Client

	// json document log sink (all json is single line per channel send).
	logSink chan []byte
}

// PolicyResult is the closure of inputs and outputs for a policy, taken
// together and usually printed or sent to a log.
type PolicyResult struct {
	InputObject map[string]interface{} `json:"InputObject"`
	ResultSet   *rego.ResultSet        `json:"ResultSet"`
	ActionTime  time.Time              `json:"ActionTime"`
}

// init takes a json document configuration and sets up the effect.
func (ap *Policy) init(ctx context.Context, client *storage.Client,
	logSink chan []byte, config *cycler_pb.PolicyEffectConfiguration,
	statsConfig *cycler_pb.StatsConfiguration, cmdMutationAllowed bool,
	runConfigMutationAllowed bool) {

	// Set the config.
	ap.Config = *config

	// Set the GCP client.
	ap.client = client
	ap.logSink = logSink

	// Set up our bucket stats config using unmarshalled config.
	ap.PrefixStats = &Stats{}
	ap.PrefixStats.init(ctx, statsConfig)

	// Set up the prefix regex
	if ap.Config.PrefixRegexp != "" {
		ap.prefixRegexp = regexp.MustCompile(ap.Config.PrefixRegexp)
	}

	// Set up our action stats with the same config.
	ap.ActionStats = &Stats{}
	ap.ActionStats.init(ctx, statsConfig)

	var protoConfig interface{}
	var actor interface{}
	switch effectType := ap.Config.EffectConfiguration.(type) {
	case *cycler_pb.PolicyEffectConfiguration_Noop:
		ap.Effect = &effects.NoopEffect{}
		protoConfig = *ap.Config.GetNoop()
	case *cycler_pb.PolicyEffectConfiguration_Duplicate:
		ap.Effect = &effects.DuplicateEffect{}
		protoConfig = *ap.Config.GetDuplicate()
	case *cycler_pb.PolicyEffectConfiguration_Move:
		ap.Effect = &effects.MoveEffect{}
		protoConfig = *ap.Config.GetMove()
	case *cycler_pb.PolicyEffectConfiguration_Chill:
		ap.Effect = &effects.ChillEffect{}
		protoConfig = *ap.Config.GetChill()
	// Additional effects here.
	// ...

	case nil:
		glog.Errorf("Effect configuration type not set: %v", effectType)
		os.Exit(2)
	default:
		glog.Errorf("Effect configuration type not implemented: %v", effectType)
		os.Exit(2)
	}

	actor = ap.Effect.DefaultActor()
	ap.Effect.Initialize(protoConfig, actor, runConfigMutationAllowed, cmdMutationAllowed)

	// Parse the rego expression defined.
	ap.r = rego.New(
		rego.Query("data.cycler"),
		rego.Load([]string{ap.Config.PolicyDocumentPath}, nil))
	ap.q = prepareQuery(&ctx, ap.r)
}

// prepareQuery
func prepareQuery(ctx *context.Context, r *rego.Rego) *rego.PreparedEvalQuery {
	q, err := r.PrepareForEval(*ctx)
	if err != nil {
		glog.Errorf("PolicyDocument query failed to prepare: %v\n", err)
		os.Exit(2)
	}
	return &q
}

func (ap *Policy) submitUnit(ctx context.Context, unit *AttrUnit) error {

	// Just for convenience make a local here.
	attr := unit.Attrs

	glog.V(3).Infof("submited work unit: %+v\n", attr)

	// Call the bucket stats module on each object...
	if err := ap.PrefixStats.submitUnit(ctx, attr); err != nil {
		return fmt.Errorf("error in submitUnit: %v", err)
	}

	ageDays, err := AgeInDays(attr.Created)
	if err != nil {
		return err
	}

	// Construct this annotated attr with fields you might not have in attr.
	annoAttr := map[string]interface{}{
		"ageDays": ageDays,
		"attr":    attr,
	}

	// Execute the prepared query.
	rs, err := ap.q.Eval(ctx, rego.EvalInput(annoAttr))
	if err != nil {
		return fmt.Errorf("error in query evaluation: %v", err)
	}

	// There shouldn't be more than a single result.
	if len(rs) > 1 {
		// This is fatal and indicates an issue with the query.
		return fmt.Errorf("query returned invalid number of values: %v", rs)
	}

	var act bool
	if act, err = shouldAct(&rs); err != nil {
		return fmt.Errorf("shouldAct determination returned an error: %v", err)
	}

	if act {
		// Do some effect here (e.g. move the object, archive it, delete it...).
		res, err := ap.Effect.Enact(ctx, ap.client, attr)

		if err != nil {
			return fmt.Errorf("error in Effect.Enact: %v", err)
		} else if res.HasActed() {
			glog.V(3).Infof("acted on: %+v\n%+v", rs, res)

			// This is the set of information serialized to the log.
			pres := PolicyResult{
				InputObject: annoAttr,
				ResultSet:   &rs,
				ActionTime:  time.Now(),
			}
			jpres, err := json.Marshal(pres)
			if err != nil {
				return fmt.Errorf("unable to marshall result set from rego: %v", err)
			}
			ap.logSink <- jpres

			// Submit to the action stats histogram.
			err = ap.ActionStats.submitUnit(ctx, attr)
			if err != nil {
				return fmt.Errorf("error in submitUnit: %v", err)
			}
		} else {
			return fmt.Errorf("matched but did not act on: %+v", err)
		}
	} else {
		glog.V(3).Infof("did not act on: %+v\n%+v", rs, err)
	}

	return nil
}

// shouldAct looks into the resulting ResultSet for the act binding and tests
// if it resulted in 'true'. Passing a result set of length greater than one
// is an error. We will return the value of the bound variable 'act', if the
// binding is missing\unset we will return false.
func shouldAct(rs *rego.ResultSet) (bool, error) {
	if len(*rs) != 1 {
		return false, fmt.Errorf("bad resultset length: %v", len(*rs))
	}
	for _, v := range (*rs)[0].Expressions {
		s, ok := v.Value.(map[string]interface{})
		if !ok {
			continue
		}
		act, ok := s["act"].(bool)
		if !ok {
			continue
		}
		return act, nil
	}
	return false, nil
}

func (ap *Policy) PrefixRegexp() *regexp.Regexp {
	return ap.prefixRegexp
}

func (ap *Policy) close() error {
	ap.PrefixStats.close()
	return nil
}

func (ap *Policy) jsonResult() ([]byte, error) {
	return json.Marshal(ap)
}

func (ap *Policy) textResult() string {
	s := "Policy Applications results:\n"
	s += "All Objects Iterated Stats:\n"
	s += ap.PrefixStats.textResult()
	s += "\nActed Objects Stats:\n"
	s += ap.ActionStats.textResult()
	return s
}
