package cloudwatcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"io/ioutil"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

var errExitFromLoop = errors.New("exit")

// GitWatcher is the specialized watcher for Git service
type GitWatcher struct {
	WatcherBase

	syncing uint32

	repository *git.Repository
	auth       transport.AuthMethod

	ticker      *time.Ticker
	stop        chan bool
	config      *gitConfiguration
	fileCache   map[string]*GitObject
	branchCache map[string]string // Branch name -> last commit hash
	tagCache    map[string]string
}

// GitCommit is the object that contains the info about the commit
type GitCommit struct {
	Hash        string
	Message     string
	Branch      string
	AuthorName  string
	AuthorEmail string
	Time        time.Time
}

// GitObject is the object that contains the info of the file
type GitObject struct {
	Key      string
	Size     int64
	FileMode os.FileMode
	Hash     string
	Commits  []*GitCommit
}

type gitConfiguration struct {
	Debug           Bool   `json:"debug"`
	MonitorType     string `json:"monitor_type"` // file, repo
	AuthType        string `json:"auth_type"`    // ssh, http_token, http_user_pass
	SSHPrivateKey   string `json:"ssh_pkey"`
	SSHPKeyPassword string `json:"ssh_pkey_password"`
	HTTPToken       string `json:"http_token"`
	HTTPUsername    string `json:"http_username"`
	HTTPPassword    string `json:"http_password"`
	RepoURL         string `json:"repo_url"`
	RepoBranch      string `json:"repo_branch"`
	AssembleEvents  Bool   `json:"assemble_events"`
	TempDir         string `json:"temp_dir"`
}

func newGitWatcher(dir string, interval time.Duration) (Watcher, error) {
	return &GitWatcher{
		tagCache:    make(map[string]string),
		fileCache:   make(map[string]*GitObject),
		branchCache: make(map[string]string),
		stop:        make(chan bool, 1),
		WatcherBase: WatcherBase{
			Events:      make(chan Event, 100),
			Errors:      make(chan error, 100),
			watchDir:    dir,
			pollingTime: interval,
		},
	}, nil
}

// SetConfig is used to configure the GitWatcher
func (w *GitWatcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := gitConfiguration{}
	if err := json.Unmarshal(j, &config); err != nil {
		return err
	}

	if config.MonitorType == "" {
		config.MonitorType = "repo" // setting default behaviour
	} else if !inArray(config.MonitorType, []string{"repo", "file"}) {
		return fmt.Errorf("unknown monitor_type '%s'", config.MonitorType)
	}

	if config.AuthType == "ssh" {
		_, err := os.Stat(config.SSHPrivateKey)
		if err != nil {
			return fmt.Errorf("cannot read file '%s': %s", config.SSHPrivateKey, err)
		}
	}

	if !inArray(config.AuthType, []string{"", "none", "ssh", "http_token", "http_user_pass"}) {
		return fmt.Errorf("unknown auth_type '%s'", config.AuthType)
	}

	if config.RepoURL == "" {
		return fmt.Errorf("url repository required")
	}

	if config.AuthType == "file" && config.RepoBranch == "" {
		return fmt.Errorf("branch repository required")
	}

	if config.TempDir == "" {
		dir, err := ioutil.TempDir("", "tmp_git")
		if err != nil {
			return fmt.Errorf("creating temp dir: %s", err)
		}
		config.TempDir = dir
	}

	w.config = &config
	return nil
}

// Start launches the polling process
func (w *GitWatcher) Start() error {
	if w.config == nil {
		return fmt.Errorf("configuration for Git needed")
	}

	w.ticker = time.NewTicker(w.pollingTime)
	go func() {
		// launch synchronization also the first time
		w.sync(true)
		for {
			select {
			case <-w.ticker.C:
				w.sync(false)

			case <-w.stop:
				close(w.Events)
				close(w.Errors)
				return
			}
		}
	}()
	return nil
}

// Close stop the polling process
func (w *GitWatcher) Close() {
	if w.stop != nil {
		w.stop <- true
	}
}

func (w *GitWatcher) getCachedObject(o *GitObject) *GitObject {
	if cachedObject, ok := w.fileCache[o.Key]; ok {
		return cachedObject
	}
	return nil
}

func (w *GitWatcher) sync(firstSync bool) {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&w.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&w.syncing, 0)

	err := w.updateRepo()
	if err != nil {
		w.Errors <- err
		return
	}

	// default behaviour is file
	if w.config.MonitorType == "repo" {
		w.checkCommits(firstSync)
		w.checkTags(firstSync)
	} else {
		fileList := make(map[string]*GitObject, 0)
		err := w.enumerateFiles(w.watchDir, func(obj *GitObject) bool {
			// With the first sync we need to cache all the files
			if firstSync {
				w.fileCache[obj.Key] = obj
				return true
			}

			// Store the files to check the deleted one
			fileList[obj.Key] = obj
			// Check if the object is cached by Key
			cached := w.getCachedObject(obj)
			// Object has been cached previously by Key
			if cached != nil {
				// Check if the Hash or the FileMode have been changed
				if cached.Hash != obj.Hash {
					event := Event{
						Key:    obj.Key,
						Type:   FileChanged,
						Object: obj,
					}
					w.Events <- event
				} else if cached.FileMode != obj.FileMode {
					event := Event{
						Key:    obj.Key,
						Type:   TagsChanged,
						Object: obj,
					}
					w.Events <- event
				}
			} else {
				event := Event{
					Key:    obj.Key,
					Type:   FileCreated,
					Object: obj,
				}
				w.Events <- event
			}
			w.fileCache[obj.Key] = obj
			return true
		})
		if err != nil {
			w.Errors <- err
			return
		}

		for k, o := range w.fileCache {
			if _, found := fileList[k]; !found {
				// file not found in the list...deleting it
				delete(w.fileCache, k)
				event := Event{
					Key:    o.Key,
					Type:   FileDeleted,
					Object: o,
				}
				w.Events <- event
			}
		}
	}
}

func (w *GitWatcher) checkCommits(disableNotification bool) {
	branches := make([]string, 0)
	// if RepoBranch is empty we are collecting all the branches
	if w.config.RepoBranch == "" {
		rIter, err := w.repository.Branches()
		if err != nil {
			w.Errors <- fmt.Errorf("retrieving branches: %s", err)
			return
		}
		err = rIter.ForEach(func(ref *plumbing.Reference) error {
			branches = append(branches, ref.Name().Short())
			return nil
		})
	} else {
		branches = append(branches, w.config.RepoBranch)
	}

	for _, branch := range branches {
		err := w.moveToBranch(branch)
		if err != nil {
			w.Errors <- err
			continue
		}

		// retrieving commits for the current branch
		cIter, err := w.repository.Log(&git.LogOptions{})
		if err != nil {
			w.Errors <- err
			return
		}

		commits := make([]*GitCommit, 0)
		err = cIter.ForEach(func(c *object.Commit) error {
			if v, ok := w.branchCache[branch]; ok {
				// Exit from the loop, we reached the last commit we saw previously
				if c.Hash.String() == v {
					return errExitFromLoop
				}

				commit := &GitCommit{
					Hash:        c.Hash.String(),
					Message:     c.Message,
					AuthorName:  c.Author.Name,
					AuthorEmail: c.Author.Email,
					Time:        c.Author.When,
					Branch:      branch,
				}
				commits = append(commits, commit)
			} else {
				// this is the first time we see this branch.
				// Storing only the last commit's hash because
				// we can't know from what other branch it comes
				commit := &GitCommit{
					Hash:        c.Hash.String(),
					Message:     c.Message,
					AuthorName:  c.Author.Name,
					AuthorEmail: c.Author.Email,
					Time:        c.Author.When,
					Branch:      branch,
				}
				commits = append(commits, commit)
				return errExitFromLoop
			}
			return nil
		})
		if err != nil && err != errExitFromLoop {
			w.Errors <- err
			return
		}

		if len(commits) != 0 {
			// Caching last commit
			w.branchCache[branch] = commits[0].Hash

			if disableNotification == false {
				if w.config.AssembleEvents {
					event := Event{
						Key:  "commit",
						Type: 0,
						Object: &GitObject{
							Commits: commits,
						},
					}
					w.Events <- event
				} else {
					for _, commit := range commits {
						event := Event{
							Key:  "commit",
							Type: 0,
							Object: &GitObject{
								Commits: []*GitCommit{commit},
							},
						}
						w.Events <- event
					}
				}
			}
		}
	}
}

func (w *GitWatcher) checkTags(disableNotification bool) {
	// event on Tags
	tagrefs, err := w.repository.Tags()
	if err != nil {
		w.Errors <- err
		return
	}
	tags := make([]*GitCommit, 0)
	err = tagrefs.ForEach(func(t *plumbing.Reference) error {
		if _, ok := w.tagCache[t.Name().Short()]; !ok {
			tags = append(tags, &GitCommit{
				Hash:    t.Hash().String(),
				Message: t.Name().Short(),
			})
			w.tagCache[t.Name().Short()] = t.Hash().String()
		}
		return nil
	})
	if err != nil {
		w.Errors <- err
		return
	}

	if disableNotification == false && len(tags) != 0 {
		if w.config.AssembleEvents {
			event := Event{
				Key:  "tag",
				Type: 0,
				Object: &GitObject{
					Commits: tags,
				},
			}
			w.Events <- event
		} else {
			for _, tag := range tags {
				event := Event{
					Key:  "tag",
					Type: 0,
					Object: &GitObject{
						Commits: []*GitCommit{tag},
					},
				}
				w.Events <- event
			}
		}
	}
}

func (w *GitWatcher) moveToBranch(branch string) error {
	wt, err := w.repository.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %s", err)
	}

	// Move to a precise branch
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
	})
	if err != nil {
		return fmt.Errorf("checkout of repo '%s': %s", branch, err)
	}
	return nil
}

func (w *GitWatcher) updateRepo() error {
	// tmp dir has been deleted?!?!
	if _, err := os.Stat(w.config.TempDir); os.IsNotExist(err) {
		w.repository = nil
	}

	if w.repository == nil {
		opts := &git.CloneOptions{
			URL: w.config.RepoURL,
		}

		switch w.config.AuthType {
		case "ssh":
			_, err := os.Stat(w.config.SSHPrivateKey)
			if err != nil {
				return fmt.Errorf("cannot read file '%s': %s", w.config.SSHPrivateKey, err)
			}

			publicKeys, err := ssh.NewPublicKeysFromFile("git", w.config.SSHPrivateKey, w.config.SSHPKeyPassword)
			if err != nil {
				return fmt.Errorf("loading pkeys from '%s': %s", w.config.SSHPrivateKey, err)
			}
			opts.Auth = publicKeys

		case "http_token":
			// token auth ignores the username but it cannot be empty 
			if w.config.HTTPUsername == "" {
				w.config.HTTPUsername = "token"
			}
			opts.Auth = &http.BasicAuth{
				Username: w.config.HTTPUsername,
				Password: w.config.HTTPToken,
			}

		case "http_user_pass":
			opts.Auth = &http.BasicAuth{
				Username: w.config.HTTPUsername,
				Password: w.config.HTTPPassword,
			}

		default:

		}
		w.auth = opts.Auth

		r, err := git.PlainClone(w.config.TempDir, false, opts)
		if err != nil && err == git.ErrRepositoryAlreadyExists {
			r, err = git.PlainOpen(w.config.TempDir)
		}
		if err != nil {
			return fmt.Errorf("cloning repo: %s", err)
		}
		w.repository = r
	}

	wt, err := w.repository.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %s", err)
	}

	// Update the repository
	err = wt.Pull(&git.PullOptions{
		Auth: w.auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("checkout of repo '%s': %s", w.config.RepoURL, err)
	}
	return nil
}

func (w *GitWatcher) enumerateFiles(prefix string, callback func(object *GitObject) bool) error {
	err := w.moveToBranch(w.config.RepoBranch)
	if err != nil {
		return fmt.Errorf("switching branch to '%s': %s", w.config.RepoBranch, err)
	}

	// getting head reference
	ref, err := w.repository.Head()
	if err != nil {
		return fmt.Errorf("getting head reference: %s", err)
	}

	// retrieve the commit pointed from head
	commit, err := w.repository.CommitObject(ref.Hash())
	if err != nil {
		return fmt.Errorf("retrieving commit '%s': %s", ref.Hash().String(), err)
	}

	// retrieve the tree from the commit
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("retrieving tree of '%s': %s", ref.Hash().String(), err)
	}

	// iterate files in the commit
	err = tree.Files().ForEach(func(f *object.File) error {
		if strings.HasPrefix(f.Name, prefix) || prefix != "" {
			o := &GitObject{
				Key:      f.Name,
				Size:     f.Size,
				Hash:     f.Hash.String(),
				FileMode: os.FileMode(f.Mode),
				Commits:  nil,
			}
			if callback(o) == false {
				return errExitFromLoop
			}
		}

		return nil
	})
	if err != nil && err != errExitFromLoop {
		return fmt.Errorf("looping files : %s", err)
	}

	return nil
}

func init() {
	supportedServices["git"] = newGitWatcher
}
