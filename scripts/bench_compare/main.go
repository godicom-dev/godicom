// Command bench_compare benchmarks godicom file I/O and gonetdicom C-STORE.
//
// Usage:
//
//	bench_compare              # JSON results to stdout
//	bench_compare scp          # gonetdicom SCP on ephemeral port (prints port)
//	bench_compare scu PORT N PATH
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/uid"
	"github.com/godicom-dev/gonetdicom/ae"
	"github.com/godicom-dev/gonetdicom/dimse"
)

const (
	warmup = 3
	runs   = 15
)

type Result struct {
	Task   string  `json:"task"`
	Tool   string  `json:"tool"`
	File   string  `json:"file"`
	Median float64 `json:"median_s"`
	OpsPer float64 `json:"ops_per_s"`
	Note   string  `json:"note,omitempty"`
}

func median(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

func bench(fn func()) time.Duration {
	for i := 0; i < warmup; i++ {
		fn()
	}
	s := make([]time.Duration, runs)
	for i := range s {
		t0 := time.Now()
		fn()
		s[i] = time.Since(t0)
	}
	return median(s)
}

func add(results *[]Result, task, tool, file string, d time.Duration) {
	*results = append(*results, Result{
		Task: task, Tool: tool, File: file,
		Median: d.Seconds(), OpsPer: float64(time.Second) / float64(d),
	})
}

func dcmtkBin() string {
	if v := os.Getenv("DCMTK_BIN"); v != "" {
		return v
	}
	for _, p := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if _, err := os.Stat(filepath.Join(p, "storescu")); err == nil {
			return p
		}
	}
	return "storescu" // hope PATH
}

func godicomRoot() string {
	if v := os.Getenv("GODICOM_ROOT"); v != "" {
		return v
	}
	// scripts/bench_compare → repo root
	if _, err := os.Stat("../../pydicom"); err == nil {
		wd, _ := os.Getwd()
		return filepath.Clean(filepath.Join(wd, "../.."))
	}
	wd, _ := os.Getwd()
	return wd
}

func gonetdicomRoot() string {
	if v := os.Getenv("GONETDICOM_ROOT"); v != "" {
		return v
	}
	sibling := filepath.Join(godicomRoot(), "..", "gonetdicom")
	if _, err := os.Stat(sibling); err == nil {
		return sibling
	}
	return sibling
}

func pydicomTestFiles() string {
	return filepath.Join(godicomRoot(), "pydicom", "src", "pydicom", "data", "test_files")
}

func pynetTestFiles() string {
	return filepath.Join(gonetdicomRoot(), "pynetdicom", "pynetdicom", "tests", "dicom_files")
}

func loadCStorePayload(path string) (sopClass, sopInst, ts string, dataset []byte, err error) {
	fd, err := godicom.ReadFile(path, nil)
	if err != nil {
		return "", "", "", nil, err
	}
	ts, ok := fd.TransferSyntaxUID()
	if !ok {
		ts = string(uid.ExplicitVRLittleEndian)
	}
	sopClass, _ = fd.GetString(godicom.MustTag("SOPClassUID"))
	sopInst, _ = fd.GetString(godicom.MustTag("SOPInstanceUID"))
	dataset, err = fd.Dataset.Encode(ts)
	return sopClass, sopInst, ts, dataset, err
}

func startGonetSCP(ctx context.Context, sopClass string) (net.Listener, <-chan error, *atomic.Int32) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	var count atomic.Int32
	errCh := make(chan error, 1)
	go func() {
		errCh <- ae.Serve(ctx, ln, ae.ServerConfig{
			AETitle:                  "GONETSCP",
			AcceptedAbstractSyntaxes: []string{sopClass},
			OnCStore: func(_ context.Context, _ ae.StoreRequest) uint16 {
				count.Add(1)
				return dimse.StatusSuccess
			},
		})
	}()
	time.Sleep(100 * time.Millisecond)
	return ln, errCh, &count
}

func gonetCStoreMany(ln net.Listener, ts, sopClass, sopInst string, dataset []byte, n int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	assoc, err := ae.Dial(ctx, ae.Config{
		AETitle: "GONETSCU",
		PresentationContexts: []ae.PresentationContext{{
			ID: 3, AbstractSyntax: sopClass, TransferSyntaxes: []string{ts},
		}},
	}, ln.Addr().String(), "GONETSCP")
	if err != nil {
		return err
	}
	defer assoc.Release(context.Background())
	for i := 0; i < n; i++ {
		if _, err := assoc.CStore(ctx, ae.StoreRequest{
			AffectedSOPClassUID:    sopClass,
			AffectedSOPInstanceUID: sopInst,
			Dataset:                dataset,
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "scp":
			runSCPOnly()
			return
		case "scu":
			runSCUOnly()
			return
		}
	}
	runBenchmarks()
}

func runSCPOnly() {
	path := filepath.Join(pynetTestFiles(), "CTImageStorage.dcm")
	sopClass, _, _, _, err := loadCStorePayload(path)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(ln.Addr().(*net.TCPAddr).Port)
	go func() {
		_ = ae.Serve(ctx, ln, ae.ServerConfig{
			AETitle:                  "GONETSCP",
			AcceptedAbstractSyntaxes: []string{sopClass},
			OnCStore: func(_ context.Context, _ ae.StoreRequest) uint16 {
				return dimse.StatusSuccess
			},
		})
	}()
	select {}
}

func runSCUOnly() {
	if len(os.Args) < 5 {
		panic("usage: bench_compare scu <port> <n> <path>")
	}
	port, _ := strconv.Atoi(os.Args[2])
	n, _ := strconv.Atoi(os.Args[3])
	path := os.Args[4]
	sopClass, sopInst, ts, payload, err := loadCStorePayload(path)
	if err != nil {
		panic(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	assoc, err := ae.Dial(ctx, ae.Config{
		AETitle: "GONETSCU",
		PresentationContexts: []ae.PresentationContext{{
			ID: 3, AbstractSyntax: sopClass, TransferSyntaxes: []string{ts},
		}},
	}, addr, "PYNETSCP")
	if err != nil {
		panic(err)
	}
	defer assoc.Release(context.Background())
	for i := 0; i < n; i++ {
		if _, err := assoc.CStore(ctx, ae.StoreRequest{
			AffectedSOPClassUID: sopClass, AffectedSOPInstanceUID: sopInst, Dataset: payload,
		}); err != nil {
			panic(err)
		}
	}
}

func runBenchmarks() {
	root := pydicomTestFiles()
	pynet := pynetTestFiles()
	files := map[string]string{
		"CT_small 38KB native": filepath.Join(root, "CT_small.dcm"),
		"MR_small_RLE 8KB":     filepath.Join(root, "MR_small_RLE.dcm"),
		"JPGExtended JPEG":     filepath.Join(root, "JPGExtended.dcm"),
		"MR JPEG-LS":           filepath.Join(root, "MR_small_jpeg_ls_lossless.dcm"),
		"RTImageStorage 2MB":   filepath.Join(pynet, "RTImageStorage.dcm"),
	}
	cstorePath := filepath.Join(pynet, "CTImageStorage.dcm")
	bin := dcmtkBin()

	var results []Result
	for label, path := range files {
		add(&results, "read metadata", "godicom", label, bench(func() {
			if _, err := godicom.ReadFile(path, &godicom.ReadOptions{StopBeforePixels: true}); err != nil {
				panic(err)
			}
		}))
		add(&results, "decode pixels", "godicom", label, bench(func() {
			fd, err := godicom.ReadFile(path, nil)
			if err != nil {
				panic(err)
			}
			if _, err := fd.PixelBytes(pixels.WithRaw(true)); err != nil {
				panic(err)
			}
		}))
	}

	sopClass, sopInst, ts, payload, err := loadCStorePayload(cstorePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load cstore: %v\n", err)
		os.Exit(1)
	}

	for _, n := range []int{100, 1000} {
		task := fmt.Sprintf("C-STORE ×%d", n)

		var samples []time.Duration
		for i := 0; i < 5; i++ {
			ctx, cancel := context.WithCancel(context.Background())
			ln, errCh, count := startGonetSCP(ctx, sopClass)
			t0 := time.Now()
			if err := gonetCStoreMany(ln, ts, sopClass, sopInst, payload, n); err != nil {
				panic(err)
			}
			samples = append(samples, time.Since(t0))
			if count.Load() != int32(n) {
				panic("store count mismatch")
			}
			cancel()
			<-errCh
			ln.Close()
		}
		d := median(samples)
		results = append(results, Result{Task: task, Tool: "gonetdicom loopback", File: "CTImageStorage 38KB", Median: d.Seconds(), OpsPer: float64(n) / d.Seconds()})

		samples = samples[:0]
		for i := 0; i < 5; i++ {
			ctx, cancel := context.WithCancel(context.Background())
			ln, errCh, count := startGonetSCP(ctx, sopClass)
			port := ln.Addr().(*net.TCPAddr).Port
			t0 := time.Now()
			scu := exec.Command(filepath.Join(bin, "storescu"),
				"-aec", "GONETSCP", "-aet", "DCMTKSCU",
				"127.0.0.1", strconv.Itoa(port),
				"--repeat", strconv.Itoa(n), cstorePath)
			scu.Stdout = nil
			scu.Stderr = nil
			if err := scu.Run(); err != nil {
				panic(err)
			}
			samples = append(samples, time.Since(t0))
			if count.Load() != int32(n) {
				panic("store count mismatch")
			}
			cancel()
			<-errCh
			ln.Close()
		}
		d = median(samples)
		results = append(results, Result{Task: task, Tool: "gonetdicom SCP", File: "CTImageStorage 38KB", Median: d.Seconds(), OpsPer: float64(n) / d.Seconds()})

		samples = samples[:0]
		for i := 0; i < 5; i++ {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				panic(err)
			}
			port := ln.Addr().(*net.TCPAddr).Port
			ln.Close()
			scp := exec.Command(filepath.Join(bin, "storescp"), "--ignore", strconv.Itoa(port))
			scp.Stdout = nil
			scp.Stderr = nil
			if err := scp.Start(); err != nil {
				panic(err)
			}
			time.Sleep(200 * time.Millisecond)
			t0 := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			assoc, err := ae.Dial(ctx, ae.Config{
				AETitle: "GONETSCU",
				PresentationContexts: []ae.PresentationContext{{
					ID: 3, AbstractSyntax: sopClass, TransferSyntaxes: []string{ts},
				}},
			}, "127.0.0.1:"+strconv.Itoa(port), "ANY-SCP")
			if err != nil {
				scp.Process.Kill()
				panic(err)
			}
			for j := 0; j < n; j++ {
				if _, err := assoc.CStore(ctx, ae.StoreRequest{
					AffectedSOPClassUID: sopClass, AffectedSOPInstanceUID: sopInst, Dataset: payload,
				}); err != nil {
					assoc.Release(context.Background())
					scp.Process.Kill()
					panic(err)
				}
			}
			assoc.Release(context.Background())
			cancel()
			samples = append(samples, time.Since(t0))
			scp.Process.Kill()
			scp.Wait()
		}
		d = median(samples)
		results = append(results, Result{Task: task, Tool: "gonetdicom SCU", File: "CTImageStorage 38KB", Median: d.Seconds(), OpsPer: float64(n) / d.Seconds()})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(results)
}
