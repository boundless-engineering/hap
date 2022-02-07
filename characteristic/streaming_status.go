// THIS FILE IS AUTO-GENERATED
package characteristic

const TypeStreamingStatus = "120"

type StreamingStatus struct {
	*Bytes
}

func NewStreamingStatus() *StreamingStatus {
	c := NewBytes(TypeStreamingStatus)
	c.Format = FormatTLV8
	c.Permissions = []string{PermissionRead, PermissionEvents}

	c.SetValue([]byte{})

	return &StreamingStatus{c}
}
