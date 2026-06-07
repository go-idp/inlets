package service

import (
	"github.com/go-idp/inlets/internal/server/config"
)

// SchemaFieldKind describes the UI control to render for a field.
type SchemaFieldKind string

const (
	KindString   SchemaFieldKind = "string"
	KindInt      SchemaFieldKind = "int"
	KindPort     SchemaFieldKind = "port"
	KindBool     SchemaFieldKind = "bool"
	KindEnum     SchemaFieldKind = "enum"
	KindDuration SchemaFieldKind = "duration"
	KindSecret   SchemaFieldKind = "secret"
)

// FieldDef describes a single field for the UI.
type FieldDef struct {
	Path        string           `json:"path"`
	Label       string           `json:"label"`
	Kind        SchemaFieldKind  `json:"kind"`
	Required    bool             `json:"required,omitempty"`
	HelpText    string           `json:"helpText,omitempty"`
	Placeholder string           `json:"placeholder,omitempty"`
	Min         *int             `json:"min,omitempty"`
	Max         *int             `json:"max,omitempty"`
	EnumValues  []string         `json:"enumValues,omitempty"`
	Default     any              `json:"default,omitempty"`
	Item        *FieldDef        `json:"item,omitempty"`        // list<kind>
	ValueFields []*FieldDef      `json:"valueFields,omitempty"` // kvMap value fields
}

// GroupDef describes a top-level config group rendered as a section.
type GroupDef struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Path   string      `json:"path"`
	Kind   string      `json:"kind"` // "object" | "list" | "kvMap"
	Fields []*FieldDef `json:"fields"`
}

// ConfigSchema is the UI-facing description of the config document.
type ConfigSchema struct {
	SchemaVersion int         `json:"schemaVersion"`
	Groups        []*GroupDef `json:"groups"`
}

// NewConfigSchema returns a static, pure description of the FileConfig
// shape. It does not load the file and has no side effects.
func NewConfigSchema() *ConfigSchema {
	trueVal := true
	zero := 0
	maxPort := 65535
	zeroSec := 0
	return &ConfigSchema{
		SchemaVersion: 1,
		Groups: []*GroupDef{
			{
				Key: "server", Label: "服务", Path: "", Kind: "object",
				Fields: []*FieldDef{
					{Path: "domain", Label: "公网域名", Kind: KindString, Required: true,
						Placeholder: "tunnel.example.com", HelpText: "客户端将使用 *.<domain> 作为公网入口"},
					{Path: "port", Label: "HTTP 端口", Kind: KindPort, Default: 80,
						Min: &zero, Max: &maxPort},
					{Path: "tcpPort", Label: "TCP 端口", Kind: KindPort, Default: 0,
						Min: &zero, Max: &maxPort, HelpText: "0 = 自动分配"},
					{Path: "token", Label: "共享 Token", Kind: KindSecret,
						HelpText: "老协议的 token 鉴权；凭证鉴权可留空"},
					{Path: "secure", Label: "HTTPS", Kind: KindBool, Default: trueVal,
						HelpText: "为客户端生成 https:// 公网入口"},
				},
			},
			{
				Key: "clients", Label: "客户端", Path: "clients", Kind: "list",
				Fields: []*FieldDef{
					{Path: "clients", Label: "客户端条目", Kind: KindString, Required: true,
						HelpText: "每个客户端凭 clientId/clientSecret 接入"},
				},
				// The "item" describes the shape of one client entry.
			},
			{
				Key: "clients.item", Label: "客户端条目", Path: "clients[*]", Kind: "object",
				Fields: []*FieldDef{
					{Path: "clientId", Label: "Client ID", Kind: KindString, Required: true},
					{Path: "clientSecret", Label: "Client Secret", Kind: KindSecret, Required: true},
				},
			},
			{
				Key: "tunnels.item", Label: "Tunnel 条目", Path: "clients[*].tunnels[*]", Kind: "object",
				Fields: []*FieldDef{
					{Path: "name", Label: "名称", Kind: KindString},
					{Path: "type", Label: "类型", Kind: KindEnum, Required: true, EnumValues: []string{"http", "tcp"}},
					{Path: "upstream", Label: "上游", Kind: KindString, Required: true,
						Placeholder: "127.0.0.1:8080"},
					{Path: "subDomain", Label: "子域", Kind: KindString,
						HelpText: "HTTP 隧道专属；空 = 客户端 CLI 指定"},
					{Path: "remotePort", Label: "远程端口", Kind: KindPort,
						Min: &zero, Max: &maxPort, HelpText: "TCP 隧道专属；0 = 沿用客户端 -p"},
				},
			},
			{
				Key: "bandwidth", Label: "带宽限制", Path: "bandwidthLimits", Kind: "object",
				Fields: []*FieldDef{
					{Path: "bandwidthLimits.global.upload", Label: "全局上行 (字节/秒)", Kind: KindInt, Min: &zeroSec},
					{Path: "bandwidthLimits.global.download", Label: "全局下行 (字节/秒)", Kind: KindInt, Min: &zeroSec},
				},
			},
			{
				Key: "notification", Label: "通知", Path: "notification", Kind: "object",
				Fields: []*FieldDef{
					{Path: "notification.provider", Label: "Provider", Kind: KindEnum,
						EnumValues: []string{"feishu", "slack", "webhook"}},
					{Path: "notification.url", Label: "Webhook URL", Kind: KindString},
				},
			},
			{
				Key: "publicHTTPNoAuth", Label: "公开 HTTP (无鉴权)", Path: "publicHTTPNoAuth", Kind: "object",
				Fields: []*FieldDef{
					{Path: "publicHTTPNoAuth.timeout", Label: "会话时长", Kind: KindDuration,
						HelpText: "例如 10m / 1h；空 = 使用默认值"},
					{Path: "publicHTTPNoAuth.warnLead", Label: "到期前提醒", Kind: KindDuration,
						HelpText: "例如 2m"},
				},
			},
			{
				Key: "admin", Label: "Admin 控制台", Path: "admin", Kind: "object",
				Fields: []*FieldDef{
					{Path: "admin.enabled", Label: "启用", Kind: KindBool},
					{Path: "admin.listen", Label: "监听地址", Kind: KindString, Placeholder: "127.0.0.1:9090"},
				},
			},
		},
	}
}

// FieldByPath returns the FieldDef for a given dotted path, or nil if not found.
func (s *ConfigSchema) FieldByPath(p string) *FieldDef {
	for _, g := range s.Groups {
		for _, f := range g.Fields {
			if f.Path == p {
				return f
			}
		}
	}
	return nil
}

// Ensure unused symbols are not pruned.
var _ = config.FileConfig{}
