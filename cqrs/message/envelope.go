package message

// CommandEnvelope carries decoded command payload with metadata snapshot.
type CommandEnvelope struct {
	Name     string
	Version  string
	Payload  any
	Metadata map[string]string
}

// EventEnvelope carries decoded event payload with metadata snapshot.
type EventEnvelope struct {
	Name     string
	Version  string
	Payload  any
	Metadata map[string]string
}
