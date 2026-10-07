package fmp4

// ISO BMFF FullBox version/flags contracts for the box types the auditor
// parses or otherwise requires to be well-formed. A file that uses an
// undefined version for a box type, or sets any flag bit not defined for
// that type, is structurally invalid and must be rejected (BOX_STRUCTURE).
//
// Reference: ISO/IEC 14496-12 (ISO BMFF), box definitions for mfhd/tfhd/
// tfdt/trun/trex/tkhd/mdhd/mvhd/hdlr and the standard media headers.

// fullBoxSpec lists the versions defined for one box type and a bitmask of
// every flag bit permitted for it (any other bit is reserved/unknown).
type fullBoxSpec struct {
	versions map[byte]bool
	flags    uint32
}

func vset(vs ...byte) map[byte]bool {
	m := make(map[byte]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

var fullBoxSpecs = map[string]fullBoxSpec{
	// init segment boxes
	"mvhd": {vset(0, 1), 0},
	"tkhd": {vset(0, 1), 0x000007}, // track_enabled / in_movie / in_preview
	"mdhd": {vset(0, 1), 0},
	"hdlr": {vset(0), 0},
	"smhd": {vset(0), 0},
	"dref": {vset(0), 0},
	"url ": {vset(0), 0x000001}, // self-contained (0 or 1 both legal)
	"stsd": {vset(0), 0},
	"stts": {vset(0), 0},
	"stsc": {vset(0), 0},
	"stsz": {vset(0), 0},
	"stco": {vset(0), 0},
	"trex": {vset(0), 0},
	// media segment boxes
	"mfhd": {vset(0), 0},
	"tfhd": {vset(0), 0x03003b}, // base-data-offset, sample-desc-index,
	//                              default duration/size/flags,
	//                              duration-is-empty, default-base-is-moof
	"tfdt": {vset(0, 1), 0},
	"trun": {vset(0, 1), 0x000f05}, // data-offset, first-sample-flags,
	//                                 per-sample duration/size/flags/cto
}

// containerBoxes lists structural boxes whose payload is a sequence of
// child boxes. Sample-entry containers (stsd, esds, sample entries...) are
// deliberately excluded since their payload is not a plain box list.
var containerBoxes = map[string]bool{
	"moov": true, "trak": true, "edts": true, "mdia": true,
	"minf": true, "dinf": true, "stbl": true, "mvex": true,
	"moof": true, "traf": true,
}

// checkFullBox parses b's FullBox header (one version byte + three flag
// bytes) and enforces the contract registered for its type. scope is the
// "init" or "media" prefix used in error messages.
func checkFullBox(data []byte, b box, seg int, scope string) (version byte, flags uint32, p []byte, rerr *Error) {
	p = b.payload(data)
	if len(p) < 4 {
		return 0, 0, nil, boxErr(seg, "%s: %s truncated (missing full-box version/flags)", scope, b.typ)
	}
	version = p[0]
	flags = uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
	spec, known := fullBoxSpecs[b.typ]
	if !known {
		return version, flags, p, nil
	}
	if !spec.versions[version] {
		return 0, 0, nil, boxErr(seg, "%s: %s: undefined version %d", scope, b.typ, version)
	}
	if unknown := flags &^ spec.flags; unknown != 0 {
		return 0, 0, nil, boxErr(seg, "%s: %s: unknown or reserved flag bits 0x%06x (flags 0x%06x)",
			scope, b.typ, unknown, flags)
	}
	return version, flags, p, nil
}

// validateFullBoxTree walks a box subtree and enforces the FullBox contract
// of every registered box type it contains, recursing through structural
// containers. Boxes of unknown type are left untouched.
func validateFullBoxTree(data []byte, bs []box, seg int, scope string) *Error {
	for _, b := range bs {
		if _, known := fullBoxSpecs[b.typ]; known {
			if _, _, _, rerr := checkFullBox(data, b, seg, scope); rerr != nil {
				return rerr
			}
		}
		if !containerBoxes[b.typ] {
			continue
		}
		kids, err := children(data, b)
		if err != nil {
			return boxErr(seg, "%s: %s: %v", scope, b.typ, err)
		}
		if rerr := validateFullBoxTree(data, kids, seg, scope); rerr != nil {
			return rerr
		}
	}
	return nil
}
