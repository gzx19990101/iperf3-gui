package engine

import (
	"os"
	"testing"
)

func TestEnsureBundled(t *testing.T) {
	path, err := EnsureBundled()
	if err != nil {
		t.Fatalf("ensure bundled: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		t.Fatalf("invalid iperf3 executable: %v %+v", err, info)
	}
	dir := path[:len(path)-len("iperf3.exe")]
	for _, name := range requiredFiles {
		info, err := os.Stat(dir + name)
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing bundled file %s: %v", name, err)
		}
	}
}
