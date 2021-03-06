package repo

// All logic for Git clone and deploy commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/daidokoro/qaz/logger"

	"golang.org/x/crypto/ssh/terminal"

	git "github.com/daidokoro/go-git"
	"github.com/daidokoro/go-git/plumbing/transport/http"
	"github.com/daidokoro/go-git/plumbing/transport/ssh"
	"github.com/daidokoro/go-git/storage/memory"
	billy "gopkg.in/src-d/go-billy.v2"
	"gopkg.in/src-d/go-billy.v2/memfs"
)

// Repo used to manage git repo based deployments
type Repo struct {
	URL    string
	fs     *memfs.Memory
	Files  map[string]string
	Config string
	RSA    string
	User   string
	Secret string
}

// Log create Logger
var Log *logger.Logger

// NewRepo - returns pointer to a new repo struct
func NewRepo(url, user, rsa string) (*Repo, error) {
	r := &Repo{
		fs:    memfs.New(),
		Files: make(map[string]string),
		URL:   url,
		RSA:   rsa,
	}

	if user != "" {
		r.User = user
	}

	if err := r.clone(); err != nil {
		return r, err
	}

	root, err := r.fs.ReadDir("/")
	if err != nil {
		return r, err
	}

	if err := r.readFiles(root, ""); err != nil {
		return r, err
	}

	return r, nil
}

func (r *Repo) clone() error {
	// memory store for git objects
	store := memory.NewStorage()

	// clone options
	opts := &git.CloneOptions{
		URL:      r.URL,
		Progress: os.Stdout,
	}

	// set authentication
	if err := r.getAuth(opts); err != nil {
		return err
	}

	Log.Debug("calling [git clone] with params: %s", opts)

	// Clones the repository into the worktree (fs) and storer all the .git
	Log.Info("fetching git repo: [%s]\n--", filepath.Base(r.URL))
	if _, err := git.Clone(store, r.fs, opts); err != nil {
		return err
	}

	fmt.Println("--")

	return nil
}

func (r *Repo) readFiles(root []billy.FileInfo, dirname string) error {
	Log.Debug("writing repo files to memory filesystem [%s]", dirname)
	for _, i := range root {
		path := filepath.Join(dirname, i.Name())
		if i.IsDir() {
			dir, _ := r.fs.ReadDir(path)

			r.readFiles(dir, path)
			continue
		}

		out, err := r.fs.Open(path)
		if err != nil {
			return err
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(out)

		// update file map
		r.Files[path] = buf.String()

	}
	return nil
}

func (r *Repo) getAuth(opts *git.CloneOptions) error {
	if strings.HasPrefix(r.URL, "git@") {
		Log.Debug("SSH Source URL detected, attempting to use SSH Keys: %s", r.RSA)

		sshAuth, err := ssh.NewPublicKeysFromFile("git", r.RSA, "")
		if err != nil {
			return err
		}

		opts.Auth = sshAuth
		return nil
	}
	if r.User != "" {
		if r.Secret == "" {
			fmt.Printf(`Password for '%s':`, r.URL)
			p, err := terminal.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return err
			}
			fmt.Printf("\n")

			r.Secret = string(p)
		}
		opts.Auth = http.NewBasicAuth(r.User, r.Secret)
	}

	return nil
}
