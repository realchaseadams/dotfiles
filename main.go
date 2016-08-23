package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
)

const (
	version = "1.0.0"
)

type Fork struct {
	Name          string
	FullName      string
	Owner         string
	DefaultBranch string
	UpstreamOwner string
}

type Repositories interface {
	List(string, *github.RepositoryListOptions) ([]*github.Repository, *github.Response, error)
	Get(string, string) (*github.Repository, *github.Response, error)
	GetBranch(string, string, string) (*github.Branch, *github.Response, error)
}

func getAllRepositories(r Repositories) ([]*github.Repository, error) {
	repos, _, err := r.List("", nil)
	if err != nil {
		return nil, fmt.Errorf("could not list repositories: %v", err)
	}
	return repos, nil
}

func updateForkUpstream(r Repositories, f *Fork) error {
	found, _, err := r.Get(f.Owner, f.Name)
	if err != nil {
		return fmt.Errorf("could not get repository: %s", err)
	}
	f.UpstreamOwner = *found.Parent.Owner.Login
	return nil
}

func getBranchSHA(r Repositories, owner, name, branch string) (string, error) {
	found, _, err := r.GetBranch(owner, name, branch)
	if err != nil {
		return "", fmt.Errorf("could not get fork branch: %s", err)
	}
	return *found.Commit.SHA, nil
}

func isForkUpToDateWithUpstream(r Repositories, f Fork) (bool, error) {
	forkSHA, err := getBranchSHA(r, f.Owner, f.Name, f.DefaultBranch)
	if err != nil {
		return false, fmt.Errorf("could not get fork branch SHA: %s", err)
	}

	upstreamSHA, err := getBranchSHA(r, f.UpstreamOwner, f.Name, f.DefaultBranch)
	if err != nil {
		return false, fmt.Errorf("could not get upstream branch SHA: %s", err)
	}

	if forkSHA == upstreamSHA {
		return true, nil
	}

	return false, nil
}

func updateFork(fork Fork, dir string) error {
	g := NewGit(fork)

	if err := g.UpdateFork(dir); err != nil {
		return fmt.Errorf("could not update fork: %v", err)
	}

	return nil

}

func newFork(r github.Repository) *Fork {
	return &Fork{
		Name:          *r.Name,
		FullName:      *r.FullName,
		Owner:         *r.Owner.Login,
		DefaultBranch: *r.DefaultBranch,
	}
}

func getForks(r Repositories) (*[]Fork, error) {
	repos, err := getAllRepositories(r)
	if err != nil {
		return nil, fmt.Errorf("could not get all repositories: %s", err)
	}

	forks := new([]Fork)

	for _, repo := range repos {
		if !*repo.Fork {
			continue
		}
		fork := newFork(*repo)

		updateForkUpstream(r, fork)
		if err != nil {
			log.Printf("could not get fork upstream for %s: %v", fork.Name, err)
			continue
		}

		*forks = append(*forks, *fork)
	}

	return forks, nil
}

func updateForks(r Repositories, forks *[]Fork) error {
	for _, fork := range *forks {
		upToDate, err := isForkUpToDateWithUpstream(r, fork)

		if upToDate {
			continue
		}

		fmt.Println(fork.Name, ":", fork.Owner, "commit does not match", fork.UpstreamOwner, "commit")

		dir, err := ioutil.TempDir("", "tmp")
		if err != nil {
			return fmt.Errorf("could not create a temporary directory: %s", err)
		}
		defer os.RemoveAll(dir)

		err = updateFork(fork, dir)
		if err != nil {
			return fmt.Errorf("could not update fork %s: %v", fork.Name, err)
		}
	}
	return nil
}

func main() {
	var (
		err   error
		token = flag.String("token", "", "The github user token to update fork on")
	)

	flag.Parse()

	if *token == "" {
		if os.Getenv("GITHUB_TOKEN") == "" {
			fmt.Fprintf(os.Stderr, "You must provide a token with the --token flag or set a GITHUB_TOKEN environment variable to use updateforks")
			os.Exit(1)
		}

		*token = os.Getenv("GITHUB_TOKEN")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)

	client := github.NewClient(tc)
	forks, err := getForks(client.Repositories)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get forks: %v", err)
		os.Exit(1)
	}

	err = updateForks(client.Repositories, forks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not update forks: %v", err)
		os.Exit(1)
	}
}
