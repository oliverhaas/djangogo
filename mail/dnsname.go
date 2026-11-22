package mail

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/idna"
)

// dnsName caches the punycode of the fully qualified name of this host,
// which names the domain of Message-IDs and the SMTP greeting.
var dnsName struct {
	sync.Mutex
	name string
	set  bool
}

// getfqdn returns the fully qualified name of this host. Tests replace it.
var getfqdn = hostFQDN

// DNSName returns the fully qualified name of this host in punycode, as
// Django's DNS_NAME does. The name is looked up once.
func DNSName() string {
	dnsName.Lock()
	defer dnsName.Unlock()
	if !dnsName.set {
		fqdn := getfqdn()
		name, err := punycode(fqdn)
		if err != nil {
			name = fqdn
		}
		dnsName.name, dnsName.set = name, true
	}
	return dnsName.name
}

// resetDNSName forgets the cached host name.
func resetDNSName() {
	dnsName.Lock()
	dnsName.set = false
	dnsName.Unlock()
}

// hostFQDN returns the first name of this host that contains a dot, as
// Python's socket.getfqdn does: the host name is resolved to an address,
// whose names are looked up in turn.
func hostFQDN() string {
	host, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return host
	}
	names, err := net.LookupAddr(addrs[0])
	if err != nil || len(names) == 0 {
		return host
	}
	for _, name := range names {
		if name = strings.TrimSuffix(name, "."); strings.Contains(name, ".") {
			return name
		}
	}
	return strings.TrimSuffix(names[0], ".")
}

// labelDots are the label separators of Python's idna codec.
var labelDots = regexp.MustCompile("[.。．｡]")

// nameprep maps a label as the nameprep step of IDNA 2003 does.
var nameprep = idna.New(idna.MapForLookup(), idna.Transitional(true), idna.StrictDomainName(false))

// punycode encodes a domain with IDNA, as Python's "idna" codec does: ASCII
// labels stay as they are, other labels are mapped and encoded with the
// "xn--" prefix.
func punycode(domain string) (string, error) {
	if domain == "" {
		return "", nil
	}
	if isASCII(domain) {
		labels := strings.Split(domain, ".")
		for _, label := range labels[:len(labels)-1] {
			if len(label) == 0 || len(label) >= 64 {
				return "", fmt.Errorf("encoding with 'idna' codec failed: label empty or too long: %s", pyRepr(domain))
			}
		}
		if len(labels[len(labels)-1]) >= 64 {
			return "", fmt.Errorf("encoding with 'idna' codec failed: label too long: %s", pyRepr(domain))
		}
		return domain, nil
	}
	labels := labelDots.Split(domain, -1)
	trailingDot := ""
	if labels[len(labels)-1] == "" {
		trailingDot = "."
		labels = labels[:len(labels)-1]
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		encoded, err := labelToASCII(label)
		if err != nil {
			return "", fmt.Errorf("encoding with 'idna' codec failed: %w: %s", err, pyRepr(domain))
		}
		out = append(out, encoded)
	}
	return strings.Join(out, ".") + trailingDot, nil
}

// labelToASCII encodes one label, as ToASCII of Python's idna codec does.
func labelToASCII(label string) (string, error) {
	if !isASCII(label) {
		mapped, err := nameprep.ToUnicode(label)
		if err != nil {
			return "", fmt.Errorf("invalid character in label %s", pyRepr(label))
		}
		label = mapped
		if !isASCII(label) {
			if strings.HasPrefix(strings.ToLower(label), "xn--") {
				return "", fmt.Errorf("label starts with ACE prefix")
			}
			if label, err = idna.Punycode.ToASCII(label); err != nil {
				return "", err
			}
		}
	}
	if len(label) == 0 || len(label) >= 64 {
		return "", fmt.Errorf("label empty or too long")
	}
	return label, nil
}
