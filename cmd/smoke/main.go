// Command smoke runs end-to-end HTTP checks against a live audit API:
// a continuous timeline must succeed and broken timelines must fail with
// the expected stable error codes. It exits non-zero on any failure.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"fmp4audit/internal/gen"
)

type segJSON struct {
	Index    int    `json:"index"`
	Sequence uint32 `json:"sequence_number"`
	Start    uint64 `json:"start"`
	End      uint64 `json:"end"`
	Samples  uint32 `json:"samples"`
	Duration uint64 `json:"duration"`
}

type okResp struct {
	OK            bool      `json:"ok"`
	Timescale     uint32    `json:"timescale"`
	TrackID       uint32    `json:"track_id"`
	SegmentCount  int       `json:"segment_count"`
	TotalDuration uint64    `json:"total_duration"`
	Segments      []segJSON `json:"segments"`
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Segment int    `json:"segment_index"`
		Message string `json:"message"`
	} `json:"error"`
}

var failures int

func pass(name string) { fmt.Printf("PASS  %s\n", name) }

func fail(name, format string, args ...any) {
	failures++
	fmt.Printf("FAIL  %s: %s\n", name, fmt.Sprintf(format, args...))
}

func postAudit(api string, init []byte, media ...[]byte) (int, []byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	parts := append([][]byte{init}, media...)
	for i, data := range parts {
		name, fn := "init", "init.mp4"
		if i > 0 {
			name, fn = "media", fmt.Sprintf("seg%04d.m4s", i)
		}
		pw, err := w.CreateFormFile(name, fn)
		if err != nil {
			return 0, nil, err
		}
		if _, err := pw.Write(data); err != nil {
			return 0, nil, err
		}
	}
	if err := w.Close(); err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, api+"/api/fmp4/audit", &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, err
}

// continuousFixture returns an init segment and three media segments whose
// decode intervals join exactly: [0,2048) [2048,3072) [3072,6144).
func continuousFixture() ([]byte, [][]byte) {
	init := gen.Init(gen.InitOpts{Timescale: 48000})
	media := [][]byte{
		gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}),
		gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: 2048, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 105}}}),
		gen.Media(gen.MediaOpts{Sequence: 3, BaseTime: 3072, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 120}, {Dur: 1024, Size: 121}, {Dur: 1024, Size: 122}}}),
	}
	return init, media
}

func expectError(api, name string, wantCode string, wantSeg int, init []byte, media ...[]byte) {
	status, body, err := postAudit(api, init, media...)
	if err != nil {
		fail(name, "request error: %v", err)
		return
	}
	if status != http.StatusUnprocessableEntity {
		fail(name, "status %d, want 422 (body: %s)", status, body)
		return
	}
	var r errResp
	if err := json.Unmarshal(body, &r); err != nil {
		fail(name, "bad error JSON: %v", err)
		return
	}
	if r.Error.Code != wantCode || r.Error.Segment != wantSeg {
		fail(name, "got %s segment %d, want %s segment %d",
			r.Error.Code, r.Error.Segment, wantCode, wantSeg)
		return
	}
	pass(name)
}

func main() {
	api := flag.String("api", "http://127.0.0.1:8080", "base URL of the audit API")
	flag.Parse()

	// 1. health endpoint
	resp, err := http.Get(*api + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		fail("health", "GET /healthz: err=%v status=%v", err, resp)
	} else {
		resp.Body.Close()
		pass("health")
	}

	// 2. continuous timeline succeeds with exact per-segment numbers
	init, media := continuousFixture()
	status, body, err := postAudit(*api, init, media...)
	if err != nil {
		fail("continuous", "request error: %v", err)
	} else if status != http.StatusOK {
		fail("continuous", "status %d, want 200 (body: %s)", status, body)
	} else {
		var r okResp
		if err := json.Unmarshal(body, &r); err != nil {
			fail("continuous", "bad success JSON: %v", err)
		} else {
			want := okResp{
				OK: true, Timescale: 48000, TrackID: 1, SegmentCount: 3, TotalDuration: 6144,
				Segments: []segJSON{
					{Index: 0, Sequence: 1, Start: 0, End: 2048, Samples: 2, Duration: 2048},
					{Index: 1, Sequence: 2, Start: 2048, End: 3072, Samples: 1, Duration: 1024},
					{Index: 2, Sequence: 3, Start: 3072, End: 6144, Samples: 3, Duration: 3072},
				},
			}
			wj, _ := json.Marshal(want)
			gj, _ := json.Marshal(r)
			if string(wj) != string(gj) {
				fail("continuous", "response mismatch:\n got %s\nwant %s", gj, wj)
			} else {
				pass("continuous")
			}
		}
	}

	// 3. timeline gap between segment 0 and 1
	_, g := continuousFixture()
	g[1] = gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: 2048 + 96, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 105}}})
	expectError(*api, "gap", "TIMELINE_GAP", 1, init, g...)

	// 4. timeline overlap between segment 0 and 1
	_, o := continuousFixture()
	o[1] = gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: 2048 - 96, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 105}}})
	expectError(*api, "overlap", "TIMELINE_OVERLAP", 1, init, o...)

	// 5. mdat payload does not match declared sample sizes
	_, p := continuousFixture()
	p[1] = gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: 2048, TrunFields: true, MdatExtra: 7,
		Samples: []gen.Sample{{Dur: 1024, Size: 105}}})
	expectError(*api, "payload", "PAYLOAD_MISMATCH", 1, init, p...)

	// 6. sequence numbers not increasing
	_, s := continuousFixture()
	s[1] = gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 2048, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 105}}})
	expectError(*api, "sequence", "SEQUENCE_NOT_INCREASING", 1, init, s...)

	// 7. non-audio init segment
	expectError(*api, "not-audio", "NOT_SINGLE_AUDIO_TRACK", -1,
		gen.Init(gen.InitOpts{Handler: "vide"}), media[0])

	// 8. init segment without mvex
	expectError(*api, "no-mvex", "NO_MVEX", -1,
		gen.Init(gen.InitOpts{OmitMvex: true}), media[0])

	// 9. normal single-segment audio (two samples, sequence 1) succeeds
	singleInit := gen.Init(gen.InitOpts{Timescale: 48000})
	singleSeg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
	status, body, err = postAudit(*api, singleInit, singleSeg)
	switch {
	case err != nil:
		fail("single-segment", "request error: %v", err)
	case status != http.StatusOK:
		fail("single-segment", "status %d, want 200 (body: %s)", status, body)
	case !strings.Contains(string(body), `"ok":true`) ||
		!strings.Contains(string(body), `"sequence_number":1`) ||
		!strings.Contains(string(body), `"samples":2`):
		fail("single-segment", "bad body: %s", body)
	default:
		pass("single-segment")
	}

	// 10. illegal FullBox versions and reserved flags are BOX_STRUCTURE
	segOpts := []struct {
		name string
		opt  gen.MediaOpts
	}{
		{"box-mfhd-version", gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}},
		{"box-mfhd-flags", gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdFlags: 0x1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}},
		{"box-tfhd-version", gen.MediaOpts{Sequence: 1, BaseTime: 0, TfhdVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}},
		{"box-tfhd-flags", gen.MediaOpts{Sequence: 1, BaseTime: 0, TfhdFlags: 0x4,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}},
		{"box-tfdt-version", gen.MediaOpts{Sequence: 1, BaseTime: 0, TfdtVersion: 2, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}}}},
		{"box-tfdt-flags", gen.MediaOpts{Sequence: 1, BaseTime: 0, TfdtFlags: 0x1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}}}},
		{"box-trun-version", gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunVersion: 2, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}}}},
		{"box-trun-flags", gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFlags: 0x2,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}}}},
	}
	for _, c := range segOpts {
		expectError(*api, c.name, "BOX_STRUCTURE", 0, singleInit, gen.Media(c.opt))
	}

	// Reserved flags on init FullBoxes are rejected at segment_index -1.
	for _, c := range []struct {
		name string
		opt  gen.InitOpts
	}{
		{"box-tkhd-flags", gen.InitOpts{TkhdFlags: 0x8}},
		{"box-mdhd-flags", gen.InitOpts{MdhdFlags: 0x1}},
		{"box-trex-version", gen.InitOpts{TrexVersion: 1}},
		{"box-mvhd-version", gen.InitOpts{MvhdVersion: 2}},
	} {
		expectError(*api, c.name, "BOX_STRUCTURE", -1, gen.Init(c.opt), singleSeg)
	}

	// 11. legal FullBox boundaries keep working: tkhd/mdhd/mvhd/tfdt/trun
	// version 1 alongside a legal mfhd version 0.
	v1Init := gen.Init(gen.InitOpts{TkhdVersion: 1, MdhdVersion: 1, MvhdVersion: 1})
	v1Segs := [][]byte{
		gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 1 << 33, TfdtVersion: 1,
			TrunVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}),
		gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: (1 << 33) + 2048, TfdtVersion: 1,
			TrunVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 105}}}),
	}
	status, body, err = postAudit(*api, v1Init, v1Segs...)
	switch {
	case err != nil:
		fail("fullbox-v1-legal", "request error: %v", err)
	case status != http.StatusOK:
		fail("fullbox-v1-legal", "status %d, want 200 (body: %s)", status, body)
	case !strings.Contains(string(body), `"ok":true`):
		fail("fullbox-v1-legal", "bad body: %s", body)
	default:
		pass("fullbox-v1-legal")
	}

	if failures > 0 {
		fmt.Printf("SMOKE FAILED: %d case(s)\n", failures)
		os.Exit(1)
	}
	fmt.Println("SMOKE OK")
}
