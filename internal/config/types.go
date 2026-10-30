package config

import "time"

// Defaults of the global flags.
const (
	// DefaultPort is the registered EtherNet/IP port of explicit messaging (TCP and UDP).
	DefaultPort = 44818
	// DefaultTimeout bounds connecting and each request.
	DefaultTimeout = 5 * time.Second
)

// Global holds the flags every command has: the persistent flags of the root command.
type Global struct {
	OutputFormat string        `env:"ETHERNETIPCTL_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Timeout      time.Duration `env:"ETHERNETIPCTL_TIMEOUT" desc:"Timeout of connecting and of each request (e.g. 500ms, 5s)" flag:"timeout"`
	Port         uint16        `env:"ETHERNETIPCTL_PORT" desc:"Port of the target when the host names none (44818)" flag:"port"`
	Debug        bool          `desc:"Log go-ethernetip's protocol trace to stderr, and show the causes of an error" flag:"debug"`
}

// NewGlobal returns the global flags at their defaults.
func NewGlobal() *Global {
	return &Global{OutputFormat: "text", Timeout: DefaultTimeout, Port: DefaultPort}
}

// RouteConfig routes requests through the target with Unconnected_Send.
type RouteConfig struct {
	Route string `env:"ETHERNETIPCTL_ROUTE" desc:"Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address" flag:"route"`
}

// MessagingConfig selects how explicit requests travel: routed, and unconnected or over a
// Class 3 connection.
type MessagingConfig struct {
	RouteConfig
	Connected bool `env:"ETHERNETIPCTL_CONNECTED" desc:"Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages" flag:"connected"`
}

// ReadConfig drives `read`.
type ReadConfig struct {
	MessagingConfig
	All  bool   `desc:"Read every attribute of the instance with Get_Attributes_All (the path is @class/instance)" flag:"all"`
	Type string `env:"ETHERNETIPCTL_TYPE" desc:"Decode the value as this CIP data type: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD, SHORT_STRING, STRING, STRING2 (an array when the data holds several)" flag:"type"`
}

// WriteConfig drives `write`.
type WriteConfig struct {
	MessagingConfig
	Data  string `desc:"Value to write as hex octets, as the target encodes it (0a00, \"0a 00\")" flag:"data"`
	Type  string `desc:"Data type of --value: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD, SHORT_STRING, STRING, STRING2" flag:"type"`
	Value string `desc:"Value to write, with --type; several comma-separated for an array" flag:"value"`
	Yes   bool   `desc:"Confirm the write: without it nothing is sent" flag:"yes"`
}

// ObjectConfig drives `objects` and `object`.
type ObjectConfig struct {
	MessagingConfig
}

// ResetConfig drives `reset`.
type ResetConfig struct {
	RouteConfig
	Type int  `desc:"Reset type of the Identity object: 0 emulates a power cycle, 1 returns to factory defaults" flag:"type"`
	Yes  bool `desc:"Confirm the reset: without it nothing is sent" flag:"yes"`
}

// SessionConfig drives `session`.
type SessionConfig struct {
	RouteConfig
	Connected bool          `env:"ETHERNETIPCTL_CONNECTED" desc:"Also open a connected (Class 3) messaging connection to the Message Router" flag:"connected"`
	Large     bool          `desc:"Open the Class 3 connection with Large_Forward_Open" flag:"large"`
	Size      uint16        `desc:"Connection size of the Class 3 connection in octets (0 = 500, 4000 with --large)" flag:"size"`
	Hold      time.Duration `desc:"Keep the session (and connection) open this long before closing it, e.g. 30s" flag:"hold"`
}

// ConnectionPathConfig names the assemblies of an I/O connection.
type ConnectionPathConfig struct {
	Config uint32 `desc:"Configuration assembly instance (0 = none in the path)" flag:"config"`
	Output uint32 `desc:"O->T connection point: the output assembly (or a heartbeat point for input-only and listen-only connections)" flag:"output"`
	Input  uint32 `desc:"T->O connection point: the input assembly" flag:"input"`
}

// IOConnConfig describes an I/O connection: its path, sizes and intervals.
type IOConnConfig struct {
	ConnectionPathConfig
	OutputSize        uint16        `desc:"Size of the output (O->T) data in octets, without sequence count and run/idle header; 0 for a heartbeat" flag:"output-size"`
	InputSize         uint16        `desc:"Size of the input (T->O) data in octets, without sequence count" flag:"input-size"`
	RPI               time.Duration `desc:"Requested packet interval of both directions, e.g. 10ms" flag:"rpi"`
	Multicast         bool          `desc:"Ask for the input (T->O) data by multicast" flag:"multicast"`
	Large             bool          `desc:"Use Large_Forward_Open (chosen anyway for sizes above 511 octets)" flag:"large"`
	TimeoutMultiplier uint8         `desc:"The connection times out after RPI x 4 << multiplier (0-7)" flag:"timeout-multiplier"`
	ConfigData        string        `desc:"Configuration data sent in the connection path, as hex (an even number of octets)" flag:"config-data"`
}

// TriadConfig is the connection triad that identifies a connection.
type TriadConfig struct {
	ConnectionSerial uint16 `desc:"Connection serial number (0 = random)" flag:"connection-serial"`
	OriginatorVendor uint16 `desc:"Originator vendor ID (0 = 0xFFFF, unregistered)" flag:"originator-vendor"`
	OriginatorSerial uint32 `desc:"Originator serial number (0 = random)" flag:"originator-serial"`
}

// ForwardOpenConfig drives `forward-open`.
type ForwardOpenConfig struct {
	RouteConfig
	IOConnConfig
	TriadConfig
	Keep bool `desc:"Leave the connection open instead of closing it with Forward_Close: the target times it out, or forward-close closes it" flag:"keep"`
}

// ForwardCloseConfig drives `forward-close`.
type ForwardCloseConfig struct {
	RouteConfig
	ConnectionPathConfig
	TriadConfig
}

// IOConfig drives `io`.
type IOConfig struct {
	IOConnConfig
	IOAddress  string        `env:"ETHERNETIPCTL_IO_ADDRESS" desc:"Local UDP address of the I/O endpoint; a port other than 2222 is announced to the target in Forward_Open" flag:"io-address"`
	Duration   time.Duration `desc:"Stop after this long (0 = until interrupted)" flag:"duration"`
	Count      uint32        `desc:"Stop after this many input samples (0 = until interrupted)" flag:"count"`
	Changes    bool          `desc:"Only write a sample when the input data or run state changes" flag:"changes"`
	OutputData string        `desc:"Output data to send, as hex (--output-size octets; default zeros); needs --yes" flag:"output-data"`
	Idle       bool          `desc:"Send the output in idle instead of run" flag:"idle"`
	Yes        bool          `desc:"Confirm that the connection writes output data (--output-size above 0): without it nothing is sent" flag:"yes"`
}

// AssemblyConfig drives `assembly read` and `assembly write`.
type AssemblyConfig struct {
	MessagingConfig
	Data string `desc:"Data to write as hex octets (assembly write)" flag:"data"`
	Yes  bool   `desc:"Confirm the write: without it nothing is sent (assembly write)" flag:"yes"`
}

// DiscoverConfig drives `discover`.
type DiscoverConfig struct {
	Address    string        `env:"ETHERNETIPCTL_DISCOVER_ADDRESS" desc:"Where the one ListIdentity request goes: a broadcast address (255.255.255.255, 192.168.1.255) or one device, with an optional :port" flag:"address"`
	Wait       time.Duration `desc:"How long replies are collected" flag:"wait"`
	Local      string        `desc:"Local IPv4 address to send from, which picks the interface" flag:"local"`
	MaxDelay   time.Duration `desc:"Largest delay a device may add to its reply (0 = the device's default)" flag:"max-delay"`
	MaxDevices int           `desc:"Stop after this many replies (0 = 1024)" flag:"max-devices"`
}

// TagReadConfig drives `tag read` and `tag decode`.
type TagReadConfig struct {
	MessagingConfig
	Count  int  `desc:"Number of elements to read" flag:"count"`
	Decode bool `desc:"Decode a structure value with its templates" flag:"decode"`
}

// TagWriteConfig drives `tag write`.
type TagWriteConfig struct {
	MessagingConfig
	Type  string `desc:"Data type of the value: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD or STRING (Logix STRING)" flag:"type"`
	Value string `desc:"Value to write; several comma-separated for consecutive array elements" flag:"value"`
	Yes   bool   `desc:"Confirm the write: without it nothing is sent" flag:"yes"`
}

// TagListConfig drives `tag list`.
type TagListConfig struct {
	MessagingConfig
	Program string `desc:"List the tags of this program (Main or Program:Main) instead of the controller-scoped tags" flag:"program"`
	System  bool   `desc:"Include system tags and programs in the list" flag:"system"`
}

// EDSGenerateConfig drives `eds generate`.
type EDSGenerateConfig struct {
	FromDevice  string   `desc:"Read the identity from this device (host[:port]) instead of the identity flags" flag:"from-device"`
	VendorID    uint16   `desc:"Vendor ID" flag:"vendor-id"`
	VendorName  string   `desc:"Vendor name (default: ODVA's name of --vendor-id)" flag:"vendor-name"`
	DeviceType  uint16   `desc:"Device type (12 = communications adapter)" flag:"device-type"`
	ProductCode uint16   `desc:"Product code" flag:"product-code"`
	Revision    string   `desc:"Major.minor revision" flag:"revision"`
	ProductName string   `desc:"Product name" flag:"product-name"`
	Description string   `desc:"Description of the file (DescText)" flag:"description"`
	Assemblies  []string `desc:"Assembly as instance:size[:name], repeatable (100:32:Input)" flag:"assembly"`
	Connections []string `desc:"I/O connection as config:output:input:output-size:input-size[:rpi[:name]], repeatable (151:150:100:32:32:10ms:Exclusive Owner)" flag:"connection"`
	Date        string   `desc:"Creation date of the file, YYYY-MM-DD or RFC 3339 (default: now)" flag:"date"`
	Output      string   `desc:"File to write (- = stdout)" flag:"output"`
	Force       bool     `desc:"Overwrite an existing --output file" flag:"force"`
}

// EDSCodegenConfig drives `eds codegen`.
type EDSCodegenConfig struct {
	Package string `desc:"Package name of the generated Go file" flag:"package"`
	Output  string `desc:"File to write (- = stdout)" flag:"output"`
	Force   bool   `desc:"Overwrite an existing --output file" flag:"force"`
}

// AdapterConfig drives `adapter`.
type AdapterConfig struct {
	Config      string        `env:"ETHERNETIPCTL_ADAPTER_CONFIG" desc:"Adapter model file (YAML or JSON); without it the flags describe the adapter" flag:"config"`
	Listen      string        `env:"ETHERNETIPCTL_LISTEN" desc:"TCP address to listen on, host[:port] (port: --port); ListIdentity is also answered on UDP at the same port" flag:"listen"`
	IOAddress   string        `env:"ETHERNETIPCTL_IO_ADDRESS" desc:"UDP address of implicit (I/O) messaging (default: port 2222 on the listen host)" flag:"io-address"`
	ProductName string        `desc:"Product name of the identity" flag:"product-name"`
	VendorID    uint16        `desc:"Vendor ID of the identity" flag:"vendor-id"`
	ProductCode uint16        `desc:"Product code of the identity" flag:"product-code"`
	Serial      uint32        `desc:"Serial number of the identity" flag:"serial"`
	Revision    string        `desc:"Revision of the identity, major.minor" flag:"revision"`
	Assemblies  []string      `desc:"Assembly as instance:kind:size, kinds input, output, config, input-only, listen-only (replaces the default set)" flag:"assembly"`
	Echo        bool          `desc:"Copy the data written to each output assembly to the input assembly of the same size with the lowest instance" flag:"echo"`
	Logix       bool          `desc:"Emulate a Logix tag database with a few demo tags (Rockwell, vendor specific)" flag:"logix"`
	Duration    time.Duration `desc:"Stop after this long (0 = until interrupted)" flag:"duration"`
	Requests    bool          `desc:"Also log every explicit request as an event" flag:"requests"`
}
