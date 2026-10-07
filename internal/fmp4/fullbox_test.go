package fmp4_test

import (
	"testing"

	"fmp4audit/internal/fmp4"
	"fmp4audit/internal/gen"
)

// TestFullBoxVersionContracts covers the headline regression: an mfhd with
// an undefined version must be rejected with BOX_STRUCTURE at segment 0,
// instead of producing a successful segment summary.
func TestFullBoxVersionContracts(t *testing.T) {
	baseInit := gen.Init(gen.InitOpts{})
	baseSeg := func() []byte {
		return gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
	}

	// Undefined versions for media-segment FullBoxes.
	t.Run("mfhd_v1_rejected", func(t *testing.T) {
		seg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		wantErr(t, baseInit, [][]byte{seg}, fmp4.CodeBoxStructure, 0)
	})

	t.Run("mfhd_v255_rejected", func(t *testing.T) {
		seg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: 255, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		wantErr(t, baseInit, [][]byte{seg}, fmp4.CodeBoxStructure, 0)
	})

	t.Run("tfdt_v2_rejected", func(t *testing.T) {
		seg := gen.PatchFullBox(baseSeg(), "tfdt", 2, 0)
		wantErr(t, baseInit, [][]byte{seg}, fmp4.CodeBoxStructure, 0)
	})

	t.Run("tfhd_v1_rejected", func(t *testing.T) {
		seg := gen.PatchFullBox(baseSeg(), "tfhd", 1, 0x020000)
		wantErr(t, baseInit, [][]byte{seg}, fmp4.CodeBoxStructure, 0)
	})

	t.Run("trex_v1_rejected", func(t *testing.T) {
		initSeg := gen.PatchFullBox(baseInit, "trex", 1, 0)
		wantErr(t, initSeg, [][]byte{baseSeg()}, fmp4.CodeBoxStructure, -1)
	})

	// Defined versions remain accepted.
	t.Run("mfhd_v0_ok", func(t *testing.T) {
		seg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: 0, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		if _, err := fmp4.Audit(baseInit, [][]byte{seg}); err != nil {
			t.Fatalf("legal mfhd v0 rejected: %v", err)
		}
	})

	t.Run("tkhd_mdhd_v1_ok", func(t *testing.T) {
		initSeg := gen.Init(gen.InitOpts{TkhdVersion: 1, MdhdVersion: 1})
		if _, err := fmp4.Audit(initSeg, [][]byte{baseSeg()}); err != nil {
			t.Fatalf("legal v1 tkhd/mdhd rejected: %v", err)
		}
	})

	t.Run("tfdt_v1_ok", func(t *testing.T) {
		seg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 1 << 33, TfdtVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		res, err := fmp4.Audit(baseInit, [][]byte{seg})
		if err != nil {
			t.Fatalf("legal tfdt v1 rejected: %v", err)
		}
		if res.Segments[0].Start != 1<<33 {
			t.Fatalf("bad start: %d", res.Segments[0].Start)
		}
	})

	t.Run("trun_v1_ok", func(t *testing.T) {
		seg := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		if _, err := fmp4.Audit(baseInit, [][]byte{seg}); err != nil {
			t.Fatalf("legal trun v1 rejected: %v", err)
		}
	})

	// trun v0 already covered by every other test; exercise a later media
	// segment failing so segment_index attribution is checked as well.
	t.Run("bad_version_on_second_segment", func(t *testing.T) {
		s1 := baseSeg()
		s2 := gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: 2048, MfhdVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
		wantErr(t, baseInit, [][]byte{s1, s2}, fmp4.CodeBoxStructure, 1)
	})
}

// TestFullBoxReservedFlags ensures unknown/reserved flag bits are rejected
// for every registered FullBox type, while all defined flag combinations
// stay accepted.
func TestFullBoxReservedFlags(t *testing.T) {
	baseInit := gen.Init(gen.InitOpts{})
	seg := func() []byte {
		return gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 64}, {Dur: 1024, Size: 64}}})
	}

	// Media-segment boxes: a single reserved high bit in each flags field.
	mediaCases := []struct {
		name  string
		typ   string
		flags uint32
	}{
		{"mfhd_flags", "mfhd", 0x800000},
		{"tfhd_reserved_high", "tfhd", 0x800000 | 0x020000},
		{"tfhd_reserved_gap", "tfhd", 0x000004 | 0x020000},
		{"tfdt_flags", "tfdt", 0x000001},
		{"trun_reserved_high", "trun", 0x800000 | 0x000300},
		{"trun_reserved_gap", "trun", 0x000002 | 0x000300},
	}
	for _, tc := range mediaCases {
		t.Run(tc.name, func(t *testing.T) {
			bad := gen.PatchFullBox(seg(), tc.typ, 0, tc.flags)
			wantErr(t, baseInit, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
		})
	}

	// Init-segment boxes.
	initCases := []struct {
		name  string
		typ   string
		flags uint32
	}{
		{"tkhd_reserved", "tkhd", 0x000100},
		{"mdhd_flags", "mdhd", 0x000001},
		{"mvhd_flags", "mvhd", 0x000001},
		{"hdlr_flags", "hdlr", 0x000001},
		{"smhd_flags", "smhd", 0x000001},
		{"dref_flags", "dref", 0x000001},
		{"stsd_flags", "stsd", 0x000001},
		{"stts_flags", "stts", 0x000001},
		{"stsc_flags", "stsc", 0x000001},
		{"stsz_flags", "stsz", 0x000001},
		{"stco_flags", "stco", 0x000001},
		{"trex_flags", "trex", 0x000001},
	}
	for _, tc := range initCases {
		t.Run(tc.name, func(t *testing.T) {
			bad := gen.PatchFullBox(baseInit, tc.typ, 0, tc.flags)
			wantErr(t, bad, [][]byte{seg()}, fmp4.CodeBoxStructure, -1)
		})
	}

	// Defined tfhd flags (base-data-offset, sample-description-index,
	// default duration/size/flags and default-base-is-moof) stay accepted.
	t.Run("tfhd_defined_flags_ok", func(t *testing.T) {
		tfhd := gen.FullBox("tfhd", 0, 0x03003b, cat(
			u32b(1),          // track_ID
			u64b(0),          // base_data_offset
			u32b(1),          // sample_description_index
			u32b(1024),       // default_sample_duration
			u32b(5),          // default_sample_size
			u32b(0x02000000), // default_sample_flags
		))
		tfdt := gen.FullBox("tfdt", 0, 0, u32b(0))
		trun := gen.FullBox("trun", 0, 0, u32b(2))
		traf := gen.Box("traf", cat(tfhd, tfdt, trun))
		moof := gen.Box("moof", cat(gen.FullBox("mfhd", 0, 0, u32b(1)), traf))
		good := cat(moof, gen.Box("mdat", make([]byte, 10))) // 2 samples x 5 bytes
		res, err := fmp4.Audit(baseInit, [][]byte{good})
		if err != nil {
			t.Fatalf("legal tfhd flags rejected: %v", err)
		}
		if res.Segments[0].Samples != 2 || res.Segments[0].Duration != 2048 {
			t.Fatalf("bad result: %+v", res.Segments[0])
		}
	})

	// Defined trun flags (data-offset, first-sample-flags, all per-sample
	// fields) stay accepted, for both version 0 (32-bit composition offsets)
	// and version 1 (64-bit composition offsets).
	for _, trunVersion := range []byte{0, 1} {
		name := "trun_defined_flags_v0_ok"
		comp := func(v uint32) []byte { return u32b(v) }
		if trunVersion == 1 {
			name = "trun_defined_flags_v1_ok"
			comp = func(v uint32) []byte { return u64b(uint64(v)) }
		}
		t.Run(name, func(t *testing.T) {
			var rec []byte
			for i := uint32(0); i < 2; i++ {
				rec = append(rec, u32b(1024)...) // sample_duration
				rec = append(rec, u32b(5)...)    // sample_size
				rec = append(rec, u32b(0)...)    // sample_flags
				rec = append(rec, comp(0)...)    // sample_composition_time_offset
			}
			trun := gen.FullBox("trun", trunVersion, 0x000f05, cat(u32b(2), u32b(100), u32b(0), rec))
			tfhd := gen.FullBox("tfhd", 0, 0x020000, u32b(1))
			tfdt := gen.FullBox("tfdt", 0, 0, u32b(0))
			traf := gen.Box("traf", cat(tfhd, tfdt, trun))
			moof := gen.Box("moof", cat(gen.FullBox("mfhd", 0, 0, u32b(1)), traf))
			good := cat(moof, gen.Box("mdat", make([]byte, 10))) // 2 samples x 5 bytes
			res, err := fmp4.Audit(baseInit, [][]byte{good})
			if err != nil {
				t.Fatalf("legal trun v%d rejected: %v", trunVersion, err)
			}
			if res.Segments[0].Samples != 2 || res.Segments[0].Duration != 2048 {
				t.Fatalf("bad result: %+v", res.Segments[0])
			}
		})
	}
}
