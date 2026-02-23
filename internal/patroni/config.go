package patroni

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// PatroniConfig holds the template variables for the Patroni configuration file.
// Passwords must be supplied from environment variables by the caller; they are
// never hardcoded here.
type PatroniConfig struct {
	ClusterName         string
	NodeName            string
	Host                string
	PGPort              int
	DataDir             string
	EtcdHosts           string // comma-separated, e.g. "10.0.0.1:2379,10.0.0.2:2379"
	ReplicationPassword string // read from env var by caller
	SuperuserPassword   string // read from env var by caller
	RewindPassword      string // read from env var by caller
}

// EtcdConfig holds the template variables for the etcd configuration file.
type EtcdConfig struct {
	NodeName       string
	Host           string
	DataDir        string
	ClusterName    string
	InitialCluster string // "node1=http://10.0.0.1:2380,node2=http://10.0.0.2:2380"
}

// RenderPatroniConfig renders the Patroni configuration file content.
func RenderPatroniConfig(cfg PatroniConfig) (string, error) {
	return renderTemplate("templates/patroni.yml.tmpl", cfg)
}

// RenderEtcdConfig renders the etcd configuration file content.
func RenderEtcdConfig(cfg EtcdConfig) (string, error) {
	return renderTemplate("templates/etcd.yml.tmpl", cfg)
}

func renderTemplate(name string, data interface{}) (string, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", name, err)
	}
	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return buf.String(), nil
}
