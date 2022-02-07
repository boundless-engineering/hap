// THIS FILE IS AUTO-GENERATED
package characteristic

const TypeSupportedVideoRecordingConfiguration = "206"

type SupportedVideoRecordingConfiguration struct {
	*Bytes
}

func NewSupportedVideoRecordingConfiguration() *SupportedVideoRecordingConfiguration {
	c := NewBytes(TypeSupportedVideoRecordingConfiguration)
	c.Format = FormatTLV8
	c.Permissions = []string{PermissionRead, PermissionEvents}

	c.SetValue([]byte{})

	return &SupportedVideoRecordingConfiguration{c}
}
