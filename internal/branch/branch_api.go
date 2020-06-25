package branch

import (
	"fmt"
	gerritapi "github.com/andygrunwald/go-gerrit"
	"go.chromium.org/luci/common/errors"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// GerritProjectBranch contains all the details for creating a new Gerrit branch
// based on an existing one.
type GerritProjectBranch struct {
	GerritURL string
	Project   string
	Branch    string
	SrcRef    string
}

func qpsToPeriod(qps float64) time.Duration {
	if qps <= 0 {
		// some very generous default duration
		LogErr("Got qps %v, <= 0. Using a default duration instead.", qps)
		return time.Second * 10
	}
	periodSec := float64(time.Second)/qps
	return time.Duration(int64(periodSec))
}

func assertBranchDoesNotExist(authedClient *http.Client, b GerritProjectBranch) error {
	agClient, err := gerritapi.NewClient(b.GerritURL, authedClient)
	if err != nil {
		return fmt.Errorf("failed to create Gerrit client: %v", err)
	}
	info, response, err := agClient.Projects.GetBranch(b.Project, b.Branch)
	defer response.Body.Close()
	if response.StatusCode == 404 {
		// Success! We're hoping not to find a branch.
		return nil
	}
	if info != nil {
		return fmt.Errorf("branch found for %v/%v : %v", b.GerritURL, b.Project, b.Branch)
	}
	return fmt.Errorf("error %v (%v) while checking for branch %v/%v : %v", response.StatusCode, response.Status, b.GerritURL, b.Project, b.Branch)
}

// AssertBranchesDoNotExist validates that the given branches do not exist on
// the Gerrit hosts using the Gerrit API.
func AssertBranchesDoNotExistApi(authedClient *http.Client, branches []GerritProjectBranch, gerritQps float64) error {
	log.Printf(
		"Verifying that destination remote branches don't already exist for %v Gerrit repos. "+
			"Use of --force skips this check.", len(branches))
	var g errgroup.Group
	throttle := time.Tick(qpsToPeriod(gerritQps))
	for _, b := range branches {
		<-throttle
		b := b
		g.Go(func() error {
			err := assertBranchDoesNotExist(authedClient, b)
			if err != nil {
				return err
			}
			return nil
		})
	}
	return g.Wait()
}

func createRemoteBranch(authedClient *http.Client, b GerritProjectBranch, dryRun, force bool) error {
	agClient, err := gerritapi.NewClient(b.GerritURL, authedClient)
	if err != nil {
		return fmt.Errorf("failed to create Gerrit client: %v", err)
	}
	if dryRun {
		return nil
	}
	bi, resp, err := agClient.Projects.CreateBranch(b.Project, b.Branch, &gerritapi.BranchInput{Revision: b.SrcRef})
	defer resp.Body.Close()
	if err != nil {
		body, err2 := ioutil.ReadAll(resp.Body)
		if err2 != nil {
			// shouldn't happen
			return err2
		}
		if resp.StatusCode == http.StatusConflict {
			if force {
				log.Printf("Branch already exists and --force is enabled, so there's nothing to do for %v/%v : %v", b.GerritURL, b.Project, b.Branch)
				return nil
			}
			return errors.Annotate(err, "Branch already exists for %v/%v : %v, so consider trying again with --force", b.GerritURL, b.Project, b.Branch).Err()
		}
		return errors.Annotate(err, "failed to create branch. Got response %v and branch info %v", string(body), bi).Err()
	}
	return nil
}

// CreateRemoteBranches creates a bunch of branches on remote Gerrit instances
// for the specified inputs using the Gerrit API.
func CreateRemoteBranchesApi(authedClient *http.Client, branches []GerritProjectBranch, dryRun, force bool, gerritQps float64) error {
	if dryRun {
		log.Printf("Dry run (no --push): would create remote branches for %v Gerrit repos", len(branches))
		return nil
	}
	log.Printf("Creating remote branches for %v Gerrit repos. This will take a few minutes, since otherwise Gerrit would throttle us.", len(branches))
	var g errgroup.Group
	throttle := time.Tick(qpsToPeriod(gerritQps))
	for _, b := range branches {
		<-throttle
		b := b
		g.Go(func() error {
			err := createRemoteBranch(authedClient, b, dryRun, force)
			if err != nil {
				return err
			}
			return nil
		})
	}
	return g.Wait()
}

// CheckSelfGroupMembership checks if the authenticated user is in the given
// group on the given Gerrit host. It returns a bool indicating whether or
// not that's the case, or an error if the lookup fails.
func CheckSelfGroupMembership(authedClient *http.Client, gerritUrl, expectedGroup string) (bool, error) {
	agClient, err := gerritapi.NewClient(gerritUrl, authedClient)
	if err != nil {
		return false, fmt.Errorf("failed to create Gerrit client: %v", err)
	}
	groups, resp, err := agClient.Accounts.ListGroups("self")
	defer resp.Body.Close()
	if err != nil {
		return false, errors.Annotate(err, "failed to get list of Gerrit groups for self").Err()
	}
	for _, g := range *groups {
		if g.Name == expectedGroup {
			return true, nil
		}
	}
	return false, nil
}
