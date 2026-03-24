package nvueschema

// formatKey is a canonical key for a group of related OpenAPI format strings.
type formatKey int

const (
	fmtIPv4Addr formatKey = iota + 1
	fmtIPv6Addr
	fmtIPAddr // dns-server-ip-address (could be v4 or v6)
	fmtIPv4Prefix
	fmtIPv6Prefix
	fmtMAC
	fmtInterfaceName
	fmtVrfName
	fmtVlanRange
	fmtPortRange
	fmtRouteDistinguisher
	fmtRouteTarget
	fmtExtCommunity
	fmtBgpCommunity
	fmtEvpnRoute
	fmtBgpRegex
	fmtAsnRange
	fmtEsIdentifier
	fmtSegmentIdentifier
	fmtHostname
	fmtUserName
	fmtSnmpOid
	fmtSecretString
	fmtInteger
	fmtFloat
	fmtDateTime
	fmtClockDate
	fmtClockTime
	fmtGenericName
	fmtFileName
	fmtRepoURL
	fmtRepoDist
	fmtJSONPointer
	fmtClockID
	fmtSequenceID
	fmtCommand
	fmtInterval
)

// formatKeyFor returns the canonical formatKey for an OpenAPI format string,
// or 0 if the format is not recognized.
func formatKeyFor(format string) formatKey {
	if k, ok := formatToKey[format]; ok {
		return k
	}
	return 0
}

// formatToKey maps every OpenAPI format string to its canonical key.
var formatToKey = map[string]formatKey{
	// IP addresses
	"ipv4": fmtIPv4Addr, "ipv4-unicast": fmtIPv4Addr,
	"ipv4-multicast": fmtIPv4Addr, "ipv4-netmask": fmtIPv4Addr,
	"ipv6": fmtIPv6Addr, "ipv6-netmask": fmtIPv6Addr,
	"dns-server-ip-address": fmtIPAddr,

	// IP prefixes
	"ipv4-prefix": fmtIPv4Prefix, "ipv4-sub-prefix": fmtIPv4Prefix,
	"ipv4-multicast-prefix": fmtIPv4Prefix, "aggregate-ipv4-prefix": fmtIPv4Prefix,
	"ipv6-prefix": fmtIPv6Prefix, "ipv6-sub-prefix": fmtIPv6Prefix,
	"aggregate-ipv6-prefix": fmtIPv6Prefix,

	// Network identifiers
	"mac":              fmtMAC,
	"interface-name":   fmtInterfaceName, "swp-name": fmtInterfaceName,
	"bond-swp-name":    fmtInterfaceName, "transceiver-name": fmtInterfaceName,
	"bridge-name":      fmtInterfaceName,
	"vrf-name":         fmtVrfName,
	"vlan-range":       fmtVlanRange,
	"ip-port-range":    fmtPortRange,
	"route-distinguisher": fmtRouteDistinguisher,
	"route-target":     fmtRouteTarget, "route-target-any": fmtRouteTarget,
	"ext-community":    fmtExtCommunity,
	"community":        fmtBgpCommunity, "well-known-community": fmtBgpCommunity,
	"large-community":  fmtBgpCommunity,
	"evpn-route":       fmtEvpnRoute,
	"bgp-regex":        fmtBgpRegex,
	"asn-range":        fmtAsnRange,
	"es-identifier":    fmtEsIdentifier,
	"segment-identifier": fmtSegmentIdentifier,

	// Names and identifiers
	"idn-hostname": fmtHostname, "domain-name": fmtHostname,
	"user-name":    fmtUserName,
	"snmp-branch":  fmtSnmpOid, "oid": fmtSnmpOid,

	// Secrets
	"secret-string": fmtSecretString, "key-string": fmtSecretString,

	// Numeric types (encoded as string)
	"integer": fmtInteger, "integer-id": fmtInteger,
	"float":   fmtFloat, "number": fmtFloat,

	// Temporal
	"date-time":  fmtDateTime,
	"clock-date": fmtClockDate,
	"clock-time": fmtClockTime,

	// Misc strings
	"generic-name": fmtGenericName, "item-name": fmtGenericName,
	"profile-name": fmtGenericName,
	"file-name":    fmtFileName,
	"repo-url":     fmtRepoURL, "remote-url-fetch": fmtRepoURL,
	"remote-url-upload": fmtRepoURL,
	"repo-dist": fmtRepoDist, "repo-pool": fmtRepoDist,
	"json-pointer": fmtJSONPointer,
	"clock-id": fmtClockID, "ptp-port-id": fmtClockID,
	"sequence-id": fmtSequenceID,
	"command": fmtCommand, "command-path": fmtCommand,
	"interval": fmtInterval, "rate-limit": fmtInterval,
	"mss-format": fmtInterval, "string": fmtInterval,
}

// typedefEntry describes a format type with a name and validation pattern.
type typedefEntry struct {
	key     formatKey
	name    string
	pattern string
	desc    string // used by YANG typedefs
}

// typedefs is the canonical list of validated format types with their patterns.
// Output generators use this to emit type aliases, typedefs, and $defs.
var typedefs = []typedefEntry{
	{fmtMAC, "MacAddress", `^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`, "IEEE 802 MAC address"},
	{fmtInterfaceName, "InterfaceName", `^(swp|eth|bond|br|lo|vlan|peerlink|erspan|mgmt)[a-zA-Z0-9_./-]*$`, "Network interface name"},
	{fmtVrfName, "VrfName", `^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`, "VRF name"},
	{fmtVlanRange, "VlanRange", `^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`, "VLAN ID or range"},
	{fmtPortRange, "PortRange", `^[0-9]+(-[0-9]+)?$`, "TCP/UDP port or range"},
	{fmtRouteDistinguisher, "RouteDistinguisher", `^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$`, "BGP route distinguisher"},
	{fmtRouteTarget, "RouteTarget", `^(\d+\.\d+\.\d+\.\d+:\d+|\d+:\d+)$`, "BGP route target"},
	{fmtExtCommunity, "ExtCommunity", `^(rt|soo|bandwidth)\s+\S+$`, "BGP extended community"},
	{fmtBgpCommunity, "BgpCommunity", `^(\d+:\d+|no-export|no-advertise|local-AS|no-peer|blackhole|graceful-shutdown|accept-own|internet)$`, "BGP community"},
	{fmtEvpnRoute, "EvpnRoute", `^(macip|imet|prefix)$`, "EVPN route type"},
	{fmtBgpRegex, "BgpRegex", ``, "BGP regular expression"},
	{fmtAsnRange, "AsnRange", `^(\d+|\d+-\d+)(,(\d+|\d+-\d+))*$`, "ASN or range"},
	{fmtEsIdentifier, "EsIdentifier", `^([0-9A-Fa-f]{2}:){9}[0-9A-Fa-f]{2}$`, "Ethernet segment identifier"},
	{fmtSegmentIdentifier, "SegmentIdentifier", `^\d+$`, "MPLS segment identifier"},
	{fmtHostname, "Hostname", `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`, "DNS hostname"},
	{fmtUserName, "UserName", `^[a-z_][a-z0-9_-]*[$]?$`, "POSIX username"},
	{fmtSnmpOid, "SnmpOid", `^\.?(\d+\.)*\d+$`, "SNMP object identifier"},
}

// typedefByKey provides fast lookup from formatKey to typedefEntry.
var typedefByKey map[formatKey]*typedefEntry

func init() {
	typedefByKey = make(map[formatKey]*typedefEntry, len(typedefs))
	for i := range typedefs {
		typedefByKey[typedefs[i].key] = &typedefs[i]
	}
}

// typedefFor returns the typedefEntry for a format key, or nil if none exists.
func typedefFor(k formatKey) *typedefEntry {
	return typedefByKey[k]
}
