package fmp4_test

import (
	"testing"

	"fmp4audit/internal/fmp4"
	"fmp4audit/internal/gen"
)

// twoSamples builds the single-segment media from the bug report: one
// continuous sequence-1 fragment, complete sample payload, no data_offset.
func twoSamples() []byte {
	return gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
}

// TestMfhdVersion1Rejected is the reported regression: mfhd version 1 is
// undefined (only version 0 exists), so the audit must fail with
// BOX_STRUCTURE at segment 0 instead of returning a success summary.
func TestMfhdVersion1Rejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: 1, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
	wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
}

// TestMfhdUndefinedVersionsRejected covers every version other than 0.
func TestMfhdUndefinedVersionsRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	for v := 2; v <= 255; v++ {
		bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdVersion: v, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}
}

// TestMfhdReservedFlagsRejected: mfhd has no defined flags, so every bit is
// reserved and must be rejected.
func TestMfhdReservedFlagsRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	for _, flags := range []int{0x000001, 0x000002, 0x000004, 0x100000, 0x800000} {
		bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, MfhdFlags: flags, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}
}

// TestTfhdReservedFlagsRejected exercises bits not defined for tfhd.
func TestTfhdReservedFlagsRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	// Defined tfhd bits are 0x000001,0x000002,0x000008,0x000010,0x000020,
	// 0x010000,0x020000; everything else is reserved.
	for _, flags := range []int{0x000004, 0x000040, 0x000080, 0x000100, 0x000800, 0x080000, 0x800000} {
		bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TfhdFlags: flags,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}
}

// TestTfhdBadVersionRejected: tfhd only defines version 0.
func TestTfhdBadVersionRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	for _, v := range []int{1, 2, 255} {
		bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TfhdVersion: v, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}
}

// TestTfdtContractRejected keeps only versions 0/1 and zero flags.
func TestTfdtContractRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})

	// Undefined versions.
	for _, v := range []byte{2, 3, 255} {
		bad := gen.Media(gen.MediaOpts{
			Sequence: 1, BaseTime: 0, TrunFields: true,
			TfdtVersion: v,
			Samples:     []gen.Sample{{Dur: 1024, Size: 100}},
		})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}

	// Reserved flags on a legal version.
	bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TfdtFlags: 0x1, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 100}}})
	wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
}

// TestTrunContractRejected keeps versions 0/1 and only the six defined flags.
func TestTrunContractRejected(t *testing.T) {
	init := gen.Init(gen.InitOpts{})

	// Undefined version.
	bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunVersion: 2, TrunFields: true,
		Samples: []gen.Sample{{Dur: 1024, Size: 100}}})
	wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)

	// Reserved bits: 0x2 and 0x8 sit among the defined optional-field flags.
	for _, flags := range []int{0x000002, 0x000008, 0x000010, 0x000020, 0x000080, 0x001000, 0x800000} {
		bad := gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 0, TrunFlags: flags,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}})
		wantErr(t, init, [][]byte{bad}, fmp4.CodeBoxStructure, 0)
	}
}

// TestInitFullBoxContractRejected covers the init-segment FullBoxes.
func TestInitFullBoxContractRejected(t *testing.T) {
	media := [][]byte{twoSamples()}

	// Undefined versions.
	for _, mutate := range []func(*gen.InitOpts){
		func(o *gen.InitOpts) { o.MvhdVersion = 2 },
		func(o *gen.InitOpts) { o.TkhdVersion = 2 },
		func(o *gen.InitOpts) { o.MdhdVersion = 2 },
		func(o *gen.InitOpts) { o.TrexVersion = 1 },
	} {
		o := gen.InitOpts{}
		mutate(&o)
		wantErr(t, gen.Init(o), media, fmp4.CodeBoxStructure, -1)
	}

	// Reserved flag bits (tkhd only defines 0x1|0x2|0x4).
	for _, mutate := range []func(*gen.InitOpts){
		func(o *gen.InitOpts) { o.MvhdFlags = 0x1 },
		func(o *gen.InitOpts) { o.TkhdFlags = 0x8 },
		func(o *gen.InitOpts) { o.TkhdFlags = 0x7 | 0x100000 },
		func(o *gen.InitOpts) { o.MdhdFlags = 0x1 },
		func(o *gen.InitOpts) { o.TrexFlags = 0x1 },
	} {
		o := gen.InitOpts{}
		mutate(&o)
		wantErr(t, gen.Init(o), media, fmp4.CodeBoxStructure, -1)
	}
}

// TestLegalFullBoxBoundariesStillPass pins the supported formats the fix
// must not narrow: tkhd/mdhd/mvhd version 1, tfdt version 1, trun version 1
// and mfhd version 0.
func TestLegalFullBoxBoundariesStillPass(t *testing.T) {
	init := gen.Init(gen.InitOpts{TkhdVersion: 1, MdhdVersion: 1, MvhdVersion: 1})
	media := [][]byte{
		gen.Media(gen.MediaOpts{Sequence: 1, BaseTime: 1 << 33, TfdtVersion: 1,
			TrunVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 100}, {Dur: 1024, Size: 110}}}),
		gen.Media(gen.MediaOpts{Sequence: 2, BaseTime: (1 << 33) + 2048, TfdtVersion: 1,
			TrunVersion: 1, TrunFields: true,
			Samples: []gen.Sample{{Dur: 1024, Size: 105}}}),
	}
	res, err := fmp4.Audit(init, media)
	if err != nil {
		t.Fatalf("legal version-1 FullBoxes rejected: %v", err)
	}
	if res.Segments[0].Sequence != 1 || res.Segments[0].Samples != 2 ||
		res.Segments[0].Start != 1<<33 || res.Segments[1].End != (1<<33)+3072 {
		t.Fatalf("bad result: %+v", res.Segments)
	}
}

// TestDefinedTfhdTrunFlagsStillPass ensures the optional-field flags already
// supported by the inheritance chain keep working.
func TestDefinedTfhdTrunFlagsStillPass(t *testing.T) {
	init := gen.Init(gen.InitOpts{})
	// tfhd base-data-offset + defaults; trun data-offset + first-sample-flags
	// alongside the per-sample duration/size flags.
	tfhd := gen.FullBox("tfhd", 0, 0x1|0x2|0x8|0x10|0x20|0x20000,
		cat(u32b(1), u64b(0), u32b(1), u32b(1024), u32b(5), u32b(0)))
	tfdt := gen.FullBox("tfdt", 0, 0, u32b(0))
	trun := gen.FullBox("trun", 0, 0x1|0x4|0x100|0x200,
		cat(u32b(2), u32b(0), u32b(0),
			u32b(1024), u32b(5), u32b(1024), u32b(5)))
	traf := gen.Box("traf", cat(tfhd, tfdt, trun))
	moof := gen.Box("moof", cat(gen.FullBox("mfhd", 0, 0, u32b(1)), traf))
	seg := cat(moof, gen.Box("mdat", make([]byte, 10)))

	res, err := fmp4.Audit(init, [][]byte{seg})
	if err != nil {
		t.Fatalf("defined tfhd/trun flags rejected: %v", err)
	}
	if res.Segments[0].Samples != 2 || res.Segments[0].Duration != 2048 {
		t.Fatalf("bad result: %+v", res.Segments[0])
	}
}
