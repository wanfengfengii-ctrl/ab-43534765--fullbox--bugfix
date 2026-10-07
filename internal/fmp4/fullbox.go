package fmp4

// ISO BMFF FullBox types carry a one-byte version and a three-byte flags
// field right after the box header. Every type defines which versions exist
// and which flag bits may be set; anything else is a structural error rather
// than data the parser should tolerate.
//
// fullBoxSpec records, per box type, the versions the audit accepts and the
// bitmask of flag bits the spec defines. versionMask is a bit mask of the
// allowed version numbers (fits in 8 bits).
type fullBoxSpec struct {
	versionMask uint8  // bit v set => version v is defined for the box type
	flagMask    uint32 // every settable flags bit (24-bit field)
}

// fullBoxSpecs is the per-type version/flags contract for the FullBoxes the
// audit parses, per ISO/IEC 14496-12:
//
//   - mvhd, tkhd, mdhd, hdlr, stsd, stts, stsc, stsz, stco, smhd, dref:
//     version 0/1 where defined, flags reserved and must be 0, except tkhd
//     (track_enabled|track_in_movie|track_in_preview) and url /urn
//     (self-contained flag 0x1);
//   - trex/tfhd/tfdt/mfhd/trun: the movie-fragment boxes, including every
//     tfhd/trun optional-field flag the parser already honours.
var fullBoxSpecs = map[string]fullBoxSpec{
	"mfhd": {versionMask: 1 << 0, flagMask: 0}, // version 0 only, flags reserved

	"tfhd": {
		versionMask: 1 << 0,
		flagMask: 0x000001 | // base-data-offset-present
			0x000002 | // sample-description-index-present
			0x000008 | // default-sample-duration-present
			0x000010 | // default-sample-size-present
			0x000020 | // default-sample-flags-present
			0x010000 | // duration-is-empty
			0x020000, // default-base-is-moof
	},
	"tfdt": {versionMask: 1<<0 | 1<<1, flagMask: 0}, // v0/v1, flags reserved
	"trex": {versionMask: 1 << 0, flagMask: 0},
	"mehd": {versionMask: 1<<0 | 1<<1, flagMask: 0},
	"trun": {
		versionMask: 1<<0 | 1<<1,
		flagMask: 0x000001 | // data-offset-present
			0x000004 | // first-sample-flags-present
			0x000100 | // sample-duration-present
			0x000200 | // sample-size-present
			0x000400 | // sample-flags-present
			0x000800, // sample-composition-time-offset-present
	},

	"mvhd": {versionMask: 1<<0 | 1<<1, flagMask: 0},
	"tkhd": {versionMask: 1<<0 | 1<<1, flagMask: 0x000001 | 0x000002 | 0x000004},
	"mdhd": {versionMask: 1<<0 | 1<<1, flagMask: 0},
	"hdlr": {versionMask: 1 << 0, flagMask: 0},
	"smhd": {versionMask: 1 << 0, flagMask: 0},
	"dref": {versionMask: 1 << 0, flagMask: 0},
	"url ": {versionMask: 1 << 0, flagMask: 0x000001}, // self-contained
	"urn ": {versionMask: 1 << 0, flagMask: 0x000001}, // self-contained
	"stsd": {versionMask: 1 << 0, flagMask: 0},
	"stts": {versionMask: 1 << 0, flagMask: 0},
	"stsc": {versionMask: 1 << 0, flagMask: 0},
	"stsz": {versionMask: 1 << 0, flagMask: 0},
	"stco": {versionMask: 1 << 0, flagMask: 0},
}

// fullBoxContainers are plain container boxes whose whole payload is a box
// sequence. dref is a FullBox container and is special-cased in the walker.
// Only containers the audit actually descends into are listed: user-data or
// edit boxes can carry content that is not a plain box sequence, and none of
// the audited FullBox types occurs inside them.
var fullBoxContainers = map[string]bool{
	"moov": true, "trak": true, "mdia": true,
	"minf": true, "dinf": true, "stbl": true, "mvex": true,
	"moof": true, "traf": true,
}

// validateFullBoxes walks the box tree rooted at bs and rejects any box with
// a registered FullBox contract whose version is undefined or whose flags
// carry unknown/reserved bits. seg is the media segment index (-1 for init),
// used on the returned BOX_STRUCTURE error.
func validateFullBoxes(data []byte, bs []box, seg int) *Error {
	where := "init"
	if seg >= 0 {
		where = "media"
	}
	for i := range bs {
		b := bs[i]
		if spec, known := fullBoxSpecs[b.typ]; known {
			p := b.payload(data)
			if len(p) < 4 {
				return boxErr(seg, "%s: %s truncated (missing version/flags)", where, b.typ)
			}
			version := p[0]
			flags := uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
			if spec.versionMask&(1<<version) == 0 {
				return boxErr(seg, "%s: %s version %d is not defined for this box type",
					where, b.typ, version)
			}
			if unknown := flags &^ spec.flagMask; unknown != 0 {
				return boxErr(seg, "%s: %s uses unknown/reserved flags 0x%06x",
					where, b.typ, unknown)
			}
		}

		switch {
		case b.typ == "dref":
			// FullBox: 4 bytes version/flags plus a 4-byte entry count precede
			// the nested data-entry boxes.
			base := b.pos + b.hdr + 8
			child, err := parseBoxes(data, base, b.size-(b.hdr+8))
			if err != nil {
				return boxErr(seg, "%s: dref: %v", where, err)
			}
			if e := validateFullBoxes(data, child, seg); e != nil {
				return e
			}
		case fullBoxContainers[b.typ]:
			child, err := children(data, b)
			if err != nil {
				return boxErr(seg, "%s: %s: %v", where, b.typ, err)
			}
			if e := validateFullBoxes(data, child, seg); e != nil {
				return e
			}
		}
	}
	return nil
}
