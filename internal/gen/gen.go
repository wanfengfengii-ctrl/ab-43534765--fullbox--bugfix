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

// patchContainers are structural boxes whose payload is a box sequence and
// PatchFullBox may descend into.
var patchContainers = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"dinf": true, "stbl": true, "mvex": true, "moof": true, "traf": true,
}

// PatchFullBox returns a copy of data with the version/flags header bytes of
// the first box of the given type (searched recursively through structural
// containers) replaced. The result keeps the same length as the input.
func PatchFullBox(data []byte, typ string, version byte, flags uint32) []byte {
	out := append([]byte(nil), data...)
	if !patchBoxes(out, 0, len(out), typ, version, flags) {
		panic("gen: box " + typ + " not found")
	}
	return out
}

func patchBoxes(data []byte, base, size int, typ string, version byte, flags uint32) bool {
	off, end := base, base+size
	for end-off >= 8 {
		sz32 := binary.BigEndian.Uint32(data[off:])
		t := string(data[off+4 : off+8])
		hdr := 8
		sz := uint64(sz32)
		if sz32 == 1 {
			if end-off < 16 {
				return false
			}
			sz = binary.BigEndian.Uint64(data[off+8:])
			hdr = 16
		} else if sz32 == 0 {
			sz = uint64(end - off)
		}
		if sz < uint64(hdr) || off+int(sz) > end {
			return false
		}
		if t == typ && int(sz)-hdr >= 4 {
			p := off + hdr
			data[p] = version
			data[p+1] = byte(flags >> 16)
			data[p+2] = byte(flags >> 8)
			data[p+3] = byte(flags)
			return true
		}
		if patchContainers[t] {
			if patchBoxes(data, off+hdr, int(sz)-hdr, typ, version, flags) {
				return true
			}
		}
		off += int(sz)
	}
	return false
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
	TkhdVersion  byte   // tkhd version: 0 (default) or 1 (64-bit times)
	MdhdVersion  byte   // mdhd version: 0 (default) or 1 (64-bit times)
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

	mvhd := FullBox("mvhd", 0, 0, concat(
		be32(0), be32(0), be32(1000), be32(0),
		be32(0x00010000), be16(0x0100), be16(0),
		make([]byte, 8), matrix(), make([]byte, 24), be32(2),
	))
	tkhdHead := []byte{0, 0, 0, 7} // flags: track_enabled | in_movie | in_preview
	tkhdHead[0] = o.TkhdVersion
	var tkhdTimes []byte
	if o.TkhdVersion == 1 {
		tkhdTimes = concat(be64(0), be64(0)) // 64-bit creation/modification time
	} else {
		tkhdTimes = concat(be32(0), be32(0))
	}
	var tkhdDuration []byte
	if o.TkhdVersion == 1 {
		tkhdDuration = be64(0)
	} else {
		tkhdDuration = be32(0)
	}
	tkhd := Box("tkhd", concat(
		tkhdHead, tkhdTimes, be32(o.TrackID), be32(0), tkhdDuration,
		make([]byte, 8),
		be16(0), be16(0), be16(0x0100), be16(0),
		matrix(), be32(0), be32(0),
	))
	mdhdHead := []byte{0, 0, 0, 0} // version overridden below; flags must be zero
	mdhdHead[0] = o.MdhdVersion
	var mdhdTimes []byte
	if o.MdhdVersion == 1 {
		mdhdTimes = concat(be64(0), be64(0)) // 64-bit creation/modification time
	} else {
		mdhdTimes = concat(be32(0), be32(0))
	}
	var mdhdDuration []byte
	if o.MdhdVersion == 1 {
		mdhdDuration = be64(0)
	} else {
		mdhdDuration = be32(0)
	}
	mdhd := Box("mdhd", concat(
		mdhdHead, mdhdTimes, be32(o.Timescale), mdhdDuration, be16(0x55c4), be16(0),
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
		trex := FullBox("trex", 0, 0, concat(
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
	MfhdVersion  byte   // mfhd version; only 0 is defined (0 = default)
	TrunVersion  byte   // trun version; 0 (default) or 1
	Samples      []Sample
	TrunFields   bool   // write per-sample duration+size into trun
	TfhdDefaults bool   // write default-sample-duration/size into tfhd (samples must be uniform)
	TrackID      uint32 // tfhd track_ID; default 1
	OmitTfdt     bool
	OmitMdat     bool
	MdatExtra    int  // append this many stray bytes to mdat
	MdatCut      int  // drop this many bytes from the mdat tail
	WithStyp     bool // prepend a segment-type box
}

// Media builds one media segment: [styp] moof(mfhd, traf(tfhd, tfdt, trun)) mdat.
func Media(o MediaOpts) []byte {
	if o.TrackID == 0 {
		o.TrackID = 1
	}
	mfhd := FullBox("mfhd", o.MfhdVersion, 0, be32(o.Sequence))

	tfhdFlags := uint32(0x020000) // default-base-is-moof
	tfhdPayload := concat(be32(o.TrackID))
	if o.TfhdDefaults && len(o.Samples) > 0 {
		tfhdFlags |= 0x8 | 0x10
		tfhdPayload = concat(tfhdPayload, be32(o.Samples[0].Dur), be32(o.Samples[0].Size))
	}
	tfhd := FullBox("tfhd", 0, tfhdFlags, tfhdPayload)

	var tfdt []byte
	if !o.OmitTfdt {
		if o.TfdtVersion == 1 {
			tfdt = FullBox("tfdt", 1, 0, be64(o.BaseTime))
		} else {
			tfdt = FullBox("tfdt", 0, 0, be32(uint32(o.BaseTime)))
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
	trun := FullBox("trun", o.TrunVersion, trunFlags, concat(be32(uint32(len(o.Samples))), rec))

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
