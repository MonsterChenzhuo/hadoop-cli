package render

import (
	"bytes"
	"encoding/xml"
	"text/template"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

type Property struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

type configuration struct {
	XMLName    xml.Name   `xml:"configuration"`
	Properties []Property `xml:"property"`
}

// XMLSite renders Hadoop-style *-site.xml files.
func XMLSite(props []Property) (string, error) {
	cfg := configuration{Properties: props}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	buf.WriteString("\n")
	return buf.String(), nil
}

// RenderText renders a Go text/template against data.
func RenderText(tpl string, data any) (string, error) {
	t, err := template.New("t").Parse(tpl)
	if err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	return buf.String(), nil
}
