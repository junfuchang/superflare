package model

// PortBinding stores user-owned metadata for a local port.
type PortBinding struct {
	Port     int    `yaml:"port" json:"port"`
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Remark   string `yaml:"remark,omitempty" json:"remark,omitempty"`
	Hidden   bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
}

// Ports stores persistent local port metadata.
type Ports struct {
	Items []PortBinding `yaml:"ports" json:"ports"`
}

// PortInfo is the merged runtime and persistent view shown in settings/editor.
type PortInfo struct {
	Port        int    `json:"Port"`
	Protocol    string `json:"Protocol"`
	ServiceName string `json:"ServiceName,omitempty"`
	Running     bool   `json:"Running"`
	PID         int    `json:"PID,omitempty"`
	Remark      string `json:"Remark,omitempty"`
	Hidden      bool   `json:"Hidden,omitempty"`
}
