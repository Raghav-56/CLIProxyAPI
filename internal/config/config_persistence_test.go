package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestSaveConfigConcurrentLoadsPreserveManagementSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	secret := "$2a$10$test-management-hash"
	initial := []byte("remote-management:\n  secret-key: \"" + secret + "\"\nrequest-retry: 1\n# " + strings.Repeat("x", 1<<16) + "\n")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	stop := make(chan struct{})
	errors := make(chan error, 4)
	var readers sync.WaitGroup
	readers.Add(4)
	for reader := 0; reader < 4; reader++ {
		go func() {
			defer readers.Done()
			<-started
			for {
				select {
				case <-stop:
					return
				default:
				}
				snapshot, err := LoadConfig(path)
				if err != nil {
					errors <- err
					return
				}
				if snapshot.RemoteManagement.SecretKey != secret || (snapshot.RequestRetry != 1 && snapshot.RequestRetry != 2) {
					errors <- fmt.Errorf("read partial config: management secret present=%t, retry=%d", snapshot.RemoteManagement.SecretKey == secret, snapshot.RequestRetry)
					return
				}
			}
		}()
	}
	close(started)
	defer func() {
		close(stop)
		readers.Wait()
		select {
		case err := <-errors:
			t.Error(err)
		default:
		}
	}()
	for i := 0; i < 30; i++ {
		cfg.RequestRetry = 1 + i%2
		if errSave := SaveConfigPreserveComments(path, cfg); errSave != nil {
			t.Fatal(errSave)
		}
	}
}

func TestSaveConfigNestedScalarPreservesSymlinkAndPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "actual.yaml")
	link := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("# keep comment\nserver:\n  port: 8317\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("actual.yaml", link); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfigPreserveCommentsUpdateNestedScalar(link, []string{"server", "port"}, "8318"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink was replaced: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# keep comment\nserver:\n  port: \"8318\"\n" {
		t.Fatalf("unexpected config: %s", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("config permissions = %o, want 640", info.Mode().Perm())
	}
	files, err := filepath.Glob(filepath.Join(dir, ".config-*.tmp"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files left: %v, error=%v", files, err)
	}
}
