package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormDataXFDF_NestedFields(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="parent">
      <field name="child">
        <value>hello</value>
      </field>
    </field>
    <field name="check">
      <value>Off</value>
      <value>Yes</value>
    </field>
  </fields>
</xfdf>`)

	data, err := ParseFormDataXFDF(input)
	require.NoError(t, err)

	require.Contains(t, data.Fields, "parent.child")
	assert.Equal(t, []string{"hello"}, data.Fields["parent.child"])

	require.Contains(t, data.Fields, "check")
	assert.Equal(t, []string{"Off", "Yes"}, data.Fields["check"])
}

func TestFormDataFDF_RoundTrip(t *testing.T) {
	input := &FormData{
		Fields: map[string][]string{
			"Name":  {"Alice"},
			"Check": {"Off"},
			"Multi": {"One", "Two"},
		},
	}

	fdf := buildFDFFromFormData(input)
	require.NotEmpty(t, fdf)

	parsed, err := ParseFormDataFDF(fdf)
	require.NoError(t, err)

	assert.Equal(t, []string{"Alice"}, parsed.Fields["Name"])
	assert.Equal(t, []string{"Off"}, parsed.Fields["Check"])
	assert.Equal(t, []string{"One", "Two"}, parsed.Fields["Multi"])
}
