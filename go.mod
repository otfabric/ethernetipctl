module github.com/otfabric/ethernetipctl

go 1.25.0

require (
	github.com/otfabric/go-ethernetip v0.0.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/inconshreveable/mousetrap v1.1.0 // indirect

// go-ethernetip is not published yet: build against the sibling checkout. This replace
// is removed when go-ethernetip v0.1.0 is tagged.
replace github.com/otfabric/go-ethernetip => ../go-ethernetip
