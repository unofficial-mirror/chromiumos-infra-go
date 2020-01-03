// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

/*
	MASTER TODO:
		* Implement a routine nanny that keeps resurrects workers.
		* Best practices warnings about same bucket source/dest.
		* Abandoned file and prefixs should be logged (with errors).
		* Integration tests need to be written.
		* Readme and documentation needs to be created.
		* Builder, definitions, schedulers, etc.
*/

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// USAGE is printed by flags on --help.
const USAGE = `
Cycler is a tool for rapid iteration of google storage buckets.

It is move effective in buckets that utilize a delimiter to indicate a hierarchical
topology. The most common example is unix like path names where the delimiter is '/'.

It provides an interface for generic effects to be mapped on to each
discovered object. For instance, to find the 'du' like tree of object
size, or to set acls, or even copy the object into another bucket.
`

// The following are runtime stats variables.
var (
	objectsFound     int64
	objectsWorked    int64
	objectsAbandoned int64
	dirsFound        int64
	dirsAbandoned    int64
)

// The following are runtime control flow state variables.
var (
	iteratorsActive    int64 = 0
	cmdMutationAllowed bool
	cyclerInvocationID = uuid.New()
	retryCount         int
)

// AttrUnit struct tracks retries and encapsulates raw storage attrs.
type AttrUnit struct {
	Attrs    *storage.ObjectAttrs `json:"Attrs"`
	TryCount int                  `json:"TryCount"`
}

// PrefixUnit struct tracks retries and encapsulates a prefix.
type PrefixUnit struct {
	Prefix   string `json:"Prefix"`
	TryCount int    `json:"TryCount"`
}

func main() {
	// Print usage.
	flag.Usage = func() {
		fmt.Printf("%v\n", USAGE)
		flag.PrintDefaults()
		os.Exit(2)
	}

	// Loggings Flags.
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "WARNING")
	flag.Set("v", "0")

	// TODO(engeg@): When cycler is mature\prod-ready remove this.
	acceptRisk := flag.Bool("iUnderstandCyclerIsInEarlyDevelopment", false,
		"you must pass this flag to accept the risk of using cycler in its early state.")

	// Process Flags.
	// The number of concurrent worker routines.
	workerJobs := flag.Int("workerJobs", 2000, "number of object consumer jobs")

	// The number of concurrent bucket iteration worker routines.
	iterJobs := flag.Int("iterJobs", 2000, "max number of object iterator jobs")

	// Optional flag to override the bucket to operate on.
	bucket := flag.String("bucket", "", "override the bucket name to operate on (e.g. gs://newbucket).")

	// Optional flag to override the runlog URL.
	runlogURL := flag.String("runlogURL", "", "override the runlog path (e.g. gs://newbucket/logs).")

	runConfigPath := flag.String("runConfigPath", "", "the RunConfig input path "+
		"(in binary or json representation).")

	// It is important that we never exceed the prefixChannelDepth, this
	// will cause goroutines to block. If all goroutines block waiting
	// waiting for this channel to have space, then there are no routines
	// to pull them off. Essentially a deadlock occurs. This could be fixed
	// by flushing found prefixes to disk, but for right now, we just set
	// a number higher than we expect to find in any bucket we care to iterate.
	// TODO(engeg): A more complicated\foolproof method of managing channel depth.
	prefixChannelDepth := flag.Int64("prefixChannelDepth", 125000000,
		"Size of the object prefix channel.")
	workUnitChannelDepth := flag.Int64("workUnitChannelDepth", 4194304,
		"Size of the work unit channel.")

	retryCountFlag := flag.Int("retryCount", 5,
		"Number of retries for an operation on any given object.")

	prefixRoot := flag.String("prefixRoot", "",
		"the root prefix to iterate as path from root without decorations "+
			"(e.g. asubdir/anotherone), defaults to root of bucket (the empty string)")

	mutationAllowedFlag := flag.Bool("mutationAllowed", false, "Must be set if "+
		"the effect specified mutates objects.")

	jsonOutFile := flag.String("jsonOutFile", "", "set if output should be "+
		"written to a json file instead of plain text to stdout.")

	// All flags are defined. Parse the options.
	flag.Parse()

	if !*acceptRisk {
		fmt.Fprintf(os.Stderr, "Error: You must awknowledge that cycler is in early (risky) development.\n")
		flag.Usage()
		os.Exit(2)
	}

	// The commmand line mutation allowed will be checked and must match the
	// effect's input configuration's mutation allowed flag.
	cmdMutationAllowed = *mutationAllowedFlag
	retryCount = *retryCountFlag

	// Read the runConfig definition proto.
	in, err := ioutil.ReadFile(*runConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Couldn't read the --runConfigPath: %v\n", err)
		flag.Usage()
		os.Exit(2)
	}

	runConfig := &cycler_pb.RunConfig{}
	if err := proto.Unmarshal(in, runConfig); err != nil {
		// Try jsonpb.
		if err = jsonpb.Unmarshal(bytes.NewReader(in), runConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error: RunConfig couldn't be unmarshaled: %v\n", err)
			os.Exit(2)
		}
	}

	if *bucket != "" {
		fmt.Printf("Warning: Overriding bucket %v to %v\n", runConfig.Bucket, *bucket)
		if strings.HasPrefix(*bucket, "gs://") {
			runConfig.Bucket = (*bucket)[5:]
		} else {
			runConfig.Bucket = *bucket
		}
	}

	if *runlogURL != "" {
		fmt.Printf("Warning: Overriding runlog %v to %v\n", runConfig.RunLogConfiguration.DestinationUrl, *runlogURL)
		runConfig.RunLogConfiguration.DestinationUrl = *runlogURL
	}

	// Initialize GS context and client.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Google Cloud client couldn't be constructed: %v\n", err)
		os.Exit(2)
	}

	// The worker, iterator and logging wait groups.
	var wwg sync.WaitGroup
	var iwg sync.WaitGroup
	var lwg sync.WaitGroup

	// Setting up log sink.
	var runlog = &Runlog{}
	runlog.Init(*runConfig.RunLogConfiguration, client, &lwg)

	// Initialize the policy.
	pol := Policy{}
	pol.init(ctx, client, runlog.LogSink,
		runConfig.PolicyEffectConfiguration,
		runConfig.StatsConfiguration,
		cmdMutationAllowed, runConfig.MutationAllowed)

	// Print invocationID.
	glog.V(0).Infof("cycler invocation uuid: %v", cyclerInvocationID)
	glog.V(2).Infof("policy:\n\n%+v\n", pol)

	workChan := make(chan *AttrUnit, *workUnitChannelDepth)
	prefixChan := make(chan *PrefixUnit, *prefixChannelDepth)
	workerStopChan := make(chan bool, *workerJobs)
	reporterStopChan := make(chan bool, 1)
	iteratorStopChan := make(chan bool, *iterJobs)

	// Set the root prefix with the passed parameter.
	root := PrefixUnit{
		Prefix:   *prefixRoot,
		TryCount: 0,
	}

	// Start the iterator jobs by sending the root.
	prefixChan <- &root
	for j := 0; j < *iterJobs; j++ {
		iwg.Add(1)
		go prefixIterator(ctx, client, &iwg, runConfig.Bucket, "/", true, workChan,
			prefixChan, iteratorStopChan)
	}

	// Start the object attr worker jobs.
	for j := 0; j < *workerJobs; j++ {
		wwg.Add(1)
		go worker(workChan, workerStopChan, &wwg, pol)
	}

	// Start the progress reporter
	go progressReporter(reporterStopChan, workChan, prefixChan)

	// Set up signal handling.
	sigsChan := make(chan os.Signal, 1)
	signal.Notify(sigsChan, syscall.SIGINT, syscall.SIGTERM)

	// You're finished when:
	//   * You've finished a single iteration at least.
	//   * There are no open iterators making progress.
	//   * There are no prefixes on the stack.
	//   * There are no work units unprocessed.
	mainTicker := time.NewTicker(100 * time.Millisecond)
MainLoop:
	for {
		select {
		case sig := <-sigsChan:
			glog.Errorf("Signal received: %v", sig)
			break MainLoop
		case _ = <-mainTicker.C:
			// Ok, there was no prefixes, how about work units.
			if len(prefixChan) == 0 && len(workChan) == 0 {
				// Ok there wasn't any work outstanding either, but perhaps
				// we still have iterators going at the moment?
				if iteratorsActive == 0 {
					break MainLoop
				}
			}
		}
	}
	mainTicker.Stop()

	// Stop handling these signals, second sig should shut down immediately.
	signal.Stop(sigsChan)

	// Stop the iterator and worker routines.
	for j := 0; j < *iterJobs; j++ {
		iteratorStopChan <- true
	}

	for j := 0; j < *workerJobs; j++ {
		workerStopChan <- true
	}

	// Block until the gsbucket iterator process is finished.
	iwg.Wait()
	wwg.Wait()

	// We can watch the threads spin down from the iterators finishing,
	// (which is why this is after the iwg and wwg wait()s).
	reporterStopChan <- true

	// Wait for logging worker group to flush.
	runlog.Stop <- true
	lwg.Wait()

	// Print the count of errors to stderr if any.
	if dirsAbandoned > 0 || objectsAbandoned > 0 {
		glog.Errorf("Prefixes abandoned: %v, Objects abandoned: %v\n",
			dirsAbandoned, objectsAbandoned)
	}

	// Print the chosen representation of the results.
	if *jsonOutFile != "" {
		if jsonBytes, err := pol.jsonResult(); err != nil {
			glog.Errorf("json marshalling failed: %v\n", err)
		} else {
			err := ioutil.WriteFile(*jsonOutFile, jsonBytes, 0644)
			if err != nil {
				glog.Errorf("json output write failed: %v\n", err)
			}
		}
	} else {
		glog.Infoln(pol.textResult())
	}
}

// worker goroutines process messages on the work chan and call effects.
func worker(work chan *AttrUnit, stop chan bool, wg *sync.WaitGroup, pol Policy) {
	defer func() {
		if r := recover(); r != nil {
			glog.Errorf("recovered from panic (but routine is dead forever): %v", r)
			glog.Errorf("stacktrace from panic: \n%v\n", string(debug.Stack()))
		}
		glog.V(3).Infoln("Worker shutting down.")
		wg.Done()
	}()

	for {
		select {
		case unit := <-work:
			ctx := context.Background()
			if err := pol.submitUnit(ctx, unit); err != nil {
				glog.V(2).Infof("error in submitUnit: %v\nWork unit: %+v", err, unit)

				// Here is where the _actual_ retry is done. Send back to channel.
				// This has the pleasant side effect of maybe deferring the work a bit.
				if unit.TryCount < retryCount {
					unit.TryCount++
					work <- unit
				} else {
					glog.V(1).Infof("unit given up upon: %v", unit.Attrs.Name)
					atomic.AddInt64(&objectsAbandoned, 1)
				}

			} else {
				atomic.AddInt64(&objectsWorked, 1)
			}
		// If you didn't receive work, then maybe you've been told to stop.
		case <-stop:
			return
		}
	}
}

// prefixIterator is a polling function that listens to prefixChan
// and calls googlestorage with a storage.Query constructed from
// the method arguments. It places the results as *storage.ObjectAttr
// on the provided chan 'workChan'. If it discovers additional prefixes
// it will place them on 'prefixChan' as approriate. It will poll
// stop gets a message.
func prefixIterator(ctx context.Context, client *storage.Client,
	wg *sync.WaitGroup, bucket string, delimiter string, versions bool,
	workChan chan *AttrUnit, prefixChan chan *PrefixUnit,
	stop chan bool) {

	var iterDelta int64

	defer func() {
		wg.Done()
		// If we've had a panic with iterators in progress, reset the
		// overall count to match. This allows the main program loop
		// to determine when we've exhausted all objects correctly.
		if iterDelta != 0 {
			glog.Errorf("prefixIterator stopped uncleanly.")
			atomic.AddInt64(&iteratorsActive, -atomic.LoadInt64(&iterDelta))
		}
	}()

	// We track the number of active iterators, these functions help.
	decIter := func(iterDelta *int64) {
		atomic.AddInt64(&iteratorsActive, -1)
		atomic.AddInt64(iterDelta, -1)
	}
	incIter := func(iterDelta *int64) {
		atomic.AddInt64(&iteratorsActive, 1)
		atomic.AddInt64(iterDelta, 1)
	}

WorkLoop:
	for {
		select {
		case thisPrefixUnit := <-prefixChan:
			glog.V(4).Infof("prefix!: %v", thisPrefixUnit)

			// We have a prefixUnit. Now iterate until finished but defer sending any
			// of the work to the channel so that if we get an error while processing
			// we can 'roll back' the work and send this particular prefix back to the
			// queue (with retries incremented) and not operate on any object more than
			// once.
			query := storage.Query{
				Prefix:    thisPrefixUnit.Prefix,
				Delimiter: delimiter,
				Versions:  versions,
			}

			it := client.Bucket(bucket).Objects(ctx, &query)
			incIter(&iterDelta)

			prefixUnits := make([]*PrefixUnit, 0)
			attrUnits := make([]*AttrUnit, 0)

			for {
				attr, err := it.Next()
				if err == iterator.Done {
					decIter(&iterDelta)
					break
				}

				// If you've encountered an error while iterating a prefix throw away
				// the parital and send it back to the channel with retries incremented.
				if err != nil {
					glog.V(1).Infof("Error encountered iterating, current iter: %v\n", it)

					if thisPrefixUnit.TryCount < retryCount {
						thisPrefixUnit.TryCount++
						prefixChan <- thisPrefixUnit
					} else {
						atomic.AddInt64(&dirsAbandoned, 1)
						glog.V(0).Infof("Prefix abandoned!: %v\n", it)
					}

					decIter(&iterDelta)
					continue WorkLoop
				}

				if attr.Prefix != "" {
					atomic.AddInt64(&dirsFound, 1)
					// TODO(engeg@): If we've completely filled the
					// prefixChan, there is a chance this will block.
					// Further if all goroutines pile up here, we can
					// deadlock the entire run. We mitigate this by
					// making prefixChan enormous but the correct long
					// term solution would be to detect the condition
					// and flush some of prefixChan to disk temporarily
					// and rehydrate it once we've processed more of
					// the channel. This might be the case in buckets
					// with extremely wide fanout.
					prefixUnit := PrefixUnit{
						Prefix:   attr.Prefix,
						TryCount: 0,
					}
					prefixUnits = append(prefixUnits, &prefixUnit)

				} else {
					atomic.AddInt64(&objectsFound, 1)

					unit := AttrUnit{
						Attrs:    attr,
						TryCount: 0,
					}
					attrUnits = append(attrUnits, &unit)
				}
			}

			// We've iterated the prefix without error, now send work to their
			// respective work or additional prefix queues.
			for _, unit := range attrUnits {
				workChan <- unit
			}

			for _, prefix := range prefixUnits {
				prefixChan <- prefix
			}

		case <-stop:
			return
		}
	}
}

// progressReporter writes to stdout the current runtime stats of the effects.
func progressReporter(stop chan bool, workChan chan *AttrUnit,
	prefixChan chan *PrefixUnit) {
	doReport := func() {
		found := atomic.LoadInt64(&objectsFound)
		dfound := atomic.LoadInt64(&dirsFound)
		worked := atomic.LoadInt64(&objectsWorked)
		var prog float64
		if found != 0 {
			prog = (float64(worked) / float64(found)) * 100.0
		} else {
			prog = 0.0
		}
		glog.V(1).Infof("Iterated %v objects, %v directories, worked %v (%.2f%%), "+
			"active routines %v, work depth %v, prefix depth %v, "+
			"iterators active %v\n",
			found, dfound, worked, prog, runtime.NumGoroutine(),
			len(workChan), len(prefixChan), atomic.LoadInt64(&iteratorsActive))
	}

	for {
		select {
		case <-stop:
			return
		case <-time.After(1 * time.Minute):
			doReport()
		}
	}
}
