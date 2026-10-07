// Package gen builds synthetic fMP4 init and media segments used by the
// unit tests and by the HTTP smoke checker.
package gen

import "encoding/binary"

// Sample is one audio sample: duration in track timescale ticks, size in bytes.
type Sample struct {
	Dur  uint32
	Size uint32
}

func be16(v uint16) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, v); return b }
func be32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func be64(v uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, v); return b }

// Box wraps payload in a box header.
func Box(typ string, payload []byte) []byte {
	b := make([]byte, 0, 8+len(payload))
	b = append(b, be32(uint32(8+len(payload)))...)
	b = append(b, typ...)
	return append(b, payload...)
}

// FullBox wraps payload in a box header plus version/flags.
func FullBox(typ string, version byte, flags uint32, payload []byte) []byte {
	p := append([]byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}, payload...)
	return Box(typ, p)
}

func concat(bs ...[]byte) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, b...)
	}
	return out
}

func matrix() []byte {
	return concat(be32(0x00010000), be32(0), be32(0),
		be32(0), be32(0x00010000), be32(0),
		be32(0), be32(0), be32(0x40000000))
}

// InitOpts tunes the generated init segment.
type InitOpts struct {
	Timescale    uint32 // default 48000
	TrackID      uint32 // default 1
	Handler      string // default "soun"; use "vide" to build a non-audio track
	OmitMvex     bool   // drop mvex entirely
	ExtraTrack   bool   // append a second (duplicate) trak
	TrexDuration uint32 // trex default_sample_duration (0 = no default)
	TrexSize     uint32 // trex default_sample_size (0 = no default)

	// FullBox version/flags overrides. Version 0 and zero flags select the
	// spec/default encoding; nonzero values override them.
	MvhdVersion int
	MvhdFlags   int
	TkhdVersion int
	TkhdFlags   int
	MdhdVersion int
	MdhdFlags   int
	TrexVersion int
	TrexFlags   int
}

// Init builds a minimal but well-formed fMP4 init segment.
func Init(o InitOpts) []byte {
	if o.Timescale == 0 {
		o.Timescale = 48000
	}
	if o.TrackID == 0 {
		o.TrackID = 1
	}
	if o.Handler == "" {
		o.Handler = "soun"
	}

	ftyp := Box("ftyp", concat([]byte("isom"), be32(0x200), []byte("isomiso6mp41dash")))

	mvhdVer, mvhdFlags := byte(0), uint32(0)
	if o.MvhdVersion > 0 {
		mvhdVer = byte(o.MvhdVersion)
	}
	if o.MvhdFlags > 0 {
		mvhdFlags = uint32(o.MvhdFlags)
	}
	var mvhdTimes []byte // creation/modification time + duration (timescale follows)
	if mvhdVer == 1 {
		mvhdTimes = concat(be64(0), be64(0))
	} else {
		mvhdTimes = concat(be32(0), be32(0))
	}
	mvhdTail := concat(be32(1000))
	if mvhdVer == 1 {
		mvhdTail = append(mvhdTail, be64(0)...)
	} else {
		mvhdTail = append(mvhdTail, be32(0)...)
	}
	mvhd := FullBox("mvhd", mvhdVer, mvhdFlags, concat(
		mvhdTimes, mvhdTail,
		be32(0x00010000), be16(0x0100), be16(0),
		make([]byte, 8), matrix(), make([]byte, 24), be32(2),
	))

	tkhdVer, tkhdFlags := byte(0), uint32(0x7)
	if o.TkhdVersion > 0 {
		tkhdVer = byte(o.TkhdVersion)
	}
	if o.TkhdFlags > 0 {
		tkhdFlags = uint32(o.TkhdFlags)
	}
	var tkhdTimes []byte
	if tkhdVer == 1 {
		tkhdTimes = concat(be64(0), be64(0))
	} else {
		tkhdTimes = concat(be32(0), be32(0))
	}
	tkhdID := concat(be32(o.TrackID), be32(0))
	var tkhdDur []byte
	if tkhdVer == 1 {
		tkhdDur = be64(0)
	} else {
		tkhdDur = be32(0)
	}
	tkhd := FullBox("tkhd", tkhdVer, tkhdFlags, concat(
		tkhdTimes, tkhdID, tkhdDur,
		make([]byte, 8),
		be16(0), be16(0), be16(0x0100), be16(0),
		matrix(), be32(0), be32(0),
	))

	mdhdVer, mdhdFlags := byte(0), uint32(0)
	if o.MdhdVersion > 0 {
		mdhdVer = byte(o.MdhdVersion)
	}
	if o.MdhdFlags > 0 {
		mdhdFlags = uint32(o.MdhdFlags)
	}
	var mdhdTimes []byte
	if mdhdVer == 1 {
		mdhdTimes = concat(be64(0), be64(0))
	} else {
		mdhdTimes = concat(be32(0), be32(0))
	}
	var mdhdDur []byte
	if mdhdVer == 1 {
		mdhdDur = be64(0)
	} else {
		mdhdDur = be32(0)
	}
	mdhd := FullBox("mdhd", mdhdVer, mdhdFlags, concat(
		mdhdTimes, be32(o.Timescale), mdhdDur, be16(0x55c4), be16(0),
	))
	hdlr := FullBox("hdlr", 0, 0, concat(
		be32(0), []byte(o.Handler), make([]byte, 12), []byte("SoundHandler\x00"),
	))
	mp4a := concat(
		make([]byte, 6), be16(1),
		make([]byte, 8),
		be16(2), be16(16), be16(0), be16(0),
		be32(o.Timescale<<16),
	)
	stsd := FullBox("stsd", 0, 0, concat(be32(1), Box("mp4a", mp4a)))
	stbl := Box("stbl", concat(
		stsd,
		FullBox("stts", 0, 0, be32(0)),
		FullBox("stsc", 0, 0, be32(0)),
		FullBox("stsz", 0, 0, concat(be32(0), be32(0))),
		FullBox("stco", 0, 0, be32(0)),
	))
	minf := Box("minf", concat(
		FullBox("smhd", 0, 0, concat(be16(0), be16(0))),
		Box("dinf", FullBox("dref", 0, 0, concat(be32(1), FullBox("url ", 0, 1, nil)))),
		stbl,
	))
	mdia := Box("mdia", concat(mdhd, hdlr, minf))
	trak := Box("trak", concat(tkhd, mdia))

	moovChildren := [][]byte{mvhd, trak}
	if o.ExtraTrack {
		moovChildren = append(moovChildren, trak)
	}
	if !o.OmitMvex {
		trexVer, trexFlags := byte(0), uint32(0)
		if o.TrexVersion > 0 {
			trexVer = byte(o.TrexVersion)
		}
		if o.TrexFlags > 0 {
			trexFlags = uint32(o.TrexFlags)
		}
		trex := FullBox("trex", trexVer, trexFlags, concat(
			be32(o.TrackID), be32(1), be32(o.TrexDuration), be32(o.TrexSize), be32(0),
		))
		moovChildren = append(moovChildren, Box("mvex", trex))
	}
	return concat(ftyp, Box("moov", concat(moovChildren...)))
}

// MediaOpts tunes the generated media segment.
type MediaOpts struct {
	Sequence     uint32 // mfhd sequence_number
	BaseTime     uint64 // tfdt baseMediaDecodeTime
	TfdtVersion  byte   // 0 (default) or 1 (64-bit)
	Samples      []Sample
	TrunFields   bool   // write per-sample duration+size into trun
	TfhdDefaults bool   // write default-sample-duration/size into tfhd (samples must be uniform)
	TrackID      uint32 // tfhd track_ID; default 1
	OmitTfdt     bool
	OmitMdat     bool
	MdatExtra    int  // append this many stray bytes to mdat
	MdatCut      int  // drop this many bytes from the mdat tail
	WithStyp     bool // prepend a segment-type box

	// FullBox version/flags overrides. Version 0 and zero flags select the
	// default (computed) encoding; nonzero values override them.
	MfhdVersion int
	MfhdFlags   int
	TfhdVersion int
	TfhdFlags   int
	TfdtFlags   int
	TrunVersion int
	TrunFlags   int
}

// Media builds one media segment: [styp] moof(mfhd, traf(tfhd, tfdt, trun)) mdat.
func Media(o MediaOpts) []byte {
	if o.TrackID == 0 {
		o.TrackID = 1
	}
	mfhdVersion, mfhdFlags := byte(0), uint32(0)
	if o.MfhdVersion > 0 {
		mfhdVersion = byte(o.MfhdVersion)
	}
	if o.MfhdFlags > 0 {
		mfhdFlags = uint32(o.MfhdFlags)
	}
	mfhd := FullBox("mfhd", mfhdVersion, mfhdFlags, be32(o.Sequence))

	tfhdFlags := uint32(0x020000) // default-base-is-moof
	tfhdPayload := concat(be32(o.TrackID))
	if o.TfhdDefaults && len(o.Samples) > 0 {
		tfhdFlags |= 0x8 | 0x10
		tfhdPayload = concat(tfhdPayload, be32(o.Samples[0].Dur), be32(o.Samples[0].Size))
	}
	tfhdVersion := byte(0)
	if o.TfhdVersion > 0 {
		tfhdVersion = byte(o.TfhdVersion)
	}
	if o.TfhdFlags > 0 {
		tfhdFlags = uint32(o.TfhdFlags)
	}
	tfhd := FullBox("tfhd", tfhdVersion, tfhdFlags, tfhdPayload)

	var tfdt []byte
	if !o.OmitTfdt {
		tfdtFlags := uint32(0)
		if o.TfdtFlags > 0 {
			tfdtFlags = uint32(o.TfdtFlags)
		}
		if o.TfdtVersion == 1 {
			tfdt = FullBox("tfdt", 1, tfdtFlags, be64(o.BaseTime))
		} else {
			// Version 0 is the legal 32-bit form; other version bytes let
			// tests exercise the contract check (which fires before the
			// baseMediaDecodeTime field is decoded).
			tfdt = FullBox("tfdt", o.TfdtVersion, tfdtFlags, be32(uint32(o.BaseTime)))
		}
	}

	var trunFlags uint32
	var rec []byte
	if o.TrunFields {
		trunFlags = 0x100 | 0x200
		for _, s := range o.Samples {
			rec = append(rec, be32(s.Dur)...)
			rec = append(rec, be32(s.Size)...)
		}
	}
	trunVersion := byte(0)
	if o.TrunVersion > 0 {
		trunVersion = byte(o.TrunVersion)
	}
	if o.TrunFlags > 0 {
		trunFlags = uint32(o.TrunFlags)
	}
	trun := FullBox("trun", trunVersion, trunFlags, concat(be32(uint32(len(o.Samples))), rec))

	trafParts := [][]byte{tfhd}
	if tfdt != nil {
		trafParts = append(trafParts, tfdt)
	}
	traf := Box("traf", concat(append(trafParts, trun)...))
	moof := Box("moof", concat(mfhd, traf))

	payloadLen := 0
	for _, s := range o.Samples {
		payloadLen += int(s.Size)
	}
	payloadLen += o.MdatExtra - o.MdatCut
	if payloadLen < 0 {
		payloadLen = 0
	}
	payload := make([]byte, payloadLen)
	for i := range payload {
		payload[i] = byte(i * 31)
	}

	var out []byte
	if o.WithStyp {
		out = append(out, Box("styp", concat([]byte("msdh"), be32(0), []byte("msdhmsix")))...)
	}
	out = append(out, moof...)
	if !o.OmitMdat {
		out = append(out, Box("mdat", payload)...)
	}
	return out
}
