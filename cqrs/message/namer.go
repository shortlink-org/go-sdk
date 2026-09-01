package message

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"google.golang.org/protobuf/proto"
)

const (
	defaultVersion = "v1"

	// Segment count for a "<kind>.<name>" message name.
	segmentsKindName = 2
)

var versionSegment = regexp.MustCompile(`^v\d+$`)

// MessageKind distinguishes commands from events.
type MessageKind string

const (
	KindCommand MessageKind = "command"
	KindEvent   MessageKind = "event"
)

// Namer builds canonical names and topics for commands and events.
type Namer interface {
	CommandName(v any) string
	EventName(v any) string
	TopicForCommand(name string) string
	TopicForEvent(name string) string
	ServiceName() string
}

// ShortlinkNamer implements the Shortlink naming convention.
type ShortlinkNamer struct {
	serviceName string
	version     string
}

// NewShortlinkNamer creates a namer bound to a service name.
func NewShortlinkNamer(serviceName string) *ShortlinkNamer {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = defaultServiceName()
	}

	return &ShortlinkNamer{
		serviceName: normalizeSegment(serviceName),
		version:     defaultVersion,
	}
}

// ServiceName returns configured service identifier.
func (n *ShortlinkNamer) ServiceName() string {
	return n.serviceName
}

// CommandName returns fully qualified command name.
func (n *ShortlinkNamer) CommandName(v any) string {
	comps := buildNameComponents(v, n.serviceName, string(KindCommand), n.version)
	comps.Kind = string(KindCommand)

	return comps.String()
}

// EventName returns fully qualified event name following ADR-0002:
// {service}.{aggregate}.{event}.{version}
func (n *ShortlinkNamer) EventName(v any) string {
	comps := buildNameComponents(v, n.serviceName, string(KindEvent), n.version)
	// For events, Kind field is used as aggregate (ADR-0002 format)
	// If aggregate is not set, use service name as aggregate
	if comps.Kind == string(KindEvent) || comps.Kind == "" {
		// Try to extract aggregate from protobuf package or use service name
		if msg, ok := toProto(v); ok {
			full := string(proto.MessageName(msg))
			parts := strings.Split(full, ".")
			// Extract aggregate from protobuf package (e.g., domain.link.v1 -> "link")
			if len(parts) >= 3 {
				comps.Kind = normalizeSegment(parts[1]) // Use as aggregate
			}
		}
		// Fallback to service name as aggregate if not found
		if comps.Kind == string(KindEvent) || comps.Kind == "" {
			comps.Kind = n.serviceName
		}
	}

	return comps.String()
}

// TopicForCommand resolves Kafka topic name for a command.
func (n *ShortlinkNamer) TopicForCommand(name string) string {
	return TopicForCommand(name)
}

// TopicForEvent resolves Kafka topic name for an event.
func (n *ShortlinkNamer) TopicForEvent(name string) string {
	return TopicForEvent(name)
}

// NameOf extracts fully qualified name using metadata or protobuf descriptors.
func NameOf(v any) string {
	defaultNamer := NewShortlinkNamer(defaultServiceName())
	kind := inferKind(v)
	comps := buildNameComponents(v, defaultNamer.serviceName, string(kind), defaultNamer.version)

	return comps.String()
}

// TopicForEvent maps canonical name to Kafka topic.
func TopicForEvent(name string) string {
	return sanitizeTopic(name)
}

// TopicForCommand maps canonical name to Kafka topic.
func TopicForCommand(name string) string {
	return sanitizeTopic(name)
}

type nameComponents struct {
	Service string
	Kind    string
	Name    string
	Version string
}

func (c nameComponents) String() string {
	service := normalizeSegment(c.Service)
	kind := normalizeSegment(c.Kind)
	name := normalizeSegment(c.Name)
	version := normalizeVersion(c.Version)

	return strings.Join([]string{service, kind, name, version}, ".")
}

func buildNameComponents(value any, fallbackService, fallbackKind, fallbackVersion string) nameComponents {
	comps := nameComponents{
		Service: fallbackService,
		Kind:    fallbackKind,
		Name:    "",
		Version: fallbackVersion,
	}

	meta := metadataFromValue(value)
	if service := meta[MetadataServiceName]; service != "" {
		comps.Service = service
	}

	if kind := meta[MetadataMessageKind]; kind != "" {
		comps.Kind = kind
	}

	if typeName := meta[MetadataTypeName]; typeName != "" {
		assignComponentsFromQualifiedName(&comps, typeName)
	}

	if version := meta[MetadataTypeVersion]; version != "" {
		comps.Version = version
	}

	if comps.Name == "" {
		// Try to extract from protobuf descriptor.
		if msg, ok := toProto(value); ok {
			full := string(proto.MessageName(msg))
			assignComponentsFromProto(&comps, full)
		}
	}

	if comps.Name == "" {
		comps.Name = camelToSnake(typeNameOf(value))
	}

	if comps.Version == "" {
		comps.Version = fallbackVersion
	}

	if comps.Service == "" {
		comps.Service = fallbackService
	}

	if comps.Kind == "" {
		comps.Kind = fallbackKind
	}

	return comps
}

func assignComponentsFromProto(comps *nameComponents, full string) {
	if full == "" {
		return
	}

	parts := strings.Split(full, ".")

	// Extract type name from protobuf package.
	// Service is already set from namer (fallbackService), so we don't override it.
	// For events, extract aggregate from protobuf package (e.g., domain.link.v1 -> "link").
	// This ensures canonical naming per ADR-0002: {service}.{aggregate}.{event}.{version}
	if len(parts) > 0 {
		typeName := parts[len(parts)-1]
		comps.Name = camelToSnake(typeName)

		// For events, extract aggregate from protobuf package if Kind is still "event"
		// Format: domain.{aggregate}.v1.TypeName -> aggregate = parts[1]
		if comps.Kind == string(KindEvent) && len(parts) >= 3 {
			// Extract aggregate from protobuf package (second segment)
			aggregate := normalizeSegment(parts[1])
			if aggregate != "" {
				comps.Kind = aggregate
			}
			// Remove aggregate prefix from event name if present
			// e.g., "LinkCreated" -> "created" (if aggregate is "link")
			eventName := camelToSnake(typeName)
			if strings.HasPrefix(strings.ToLower(eventName), strings.ToLower(aggregate)+"_") {
				comps.Name = strings.TrimPrefix(eventName, strings.ToLower(aggregate)+"_")
			}
		}
	}

	// Only extract version if it's not already set and protobuf package has version segment
	if comps.Version == "" && len(parts) >= 3 && versionSegment.MatchString(parts[len(parts)-2]) {
		comps.Version = parts[len(parts)-2]
	}
}

func assignComponentsFromQualifiedName(comps *nameComponents, qualified string) {
	segments := strings.Split(qualified, ".")
	switch len(segments) {
	case 0:
		return
	case 1:
		comps.Name = segments[0]
	case segmentsKindName:
		comps.Kind = segments[0]
		comps.Name = segments[1]
	default:
		comps.Service = segments[0]
		comps.Kind = segments[1]
		comps.Name = segments[len(segments)-1]
	}
}

func camelToSnake(input string) string {
	if input == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(input))

	for i, char := range input {
		if unicode.IsUpper(char) {
			if i > 0 {
				builder.WriteByte('_')
			}

			builder.WriteRune(unicode.ToLower(char))
		} else {
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

func typeNameOf(value any) string {
	if value == nil {
		return ""
	}

	typ := reflect.TypeOf(value)
	if typ == nil {
		return ""
	}

	for typ.Kind() == reflect.Pointer {
		if typ.Elem() == nil {
			break
		}

		typ = typ.Elem()
	}

	return typ.Name()
}

//nolint:ireturn // proto.Message is the protobuf runtime contract.
func toProto(value any) (proto.Message, bool) {
	if value == nil {
		return nil, false
	}

	if msg, ok := value.(proto.Message); ok {
		return msg, true
	}

	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, false
	}

	if val.Kind() == reflect.Pointer && val.IsNil() {
		// Create zero instance of the pointer element.
		elem := reflect.New(val.Type().Elem())
		if msg, ok := reflect.TypeAssert[proto.Message](elem); ok {
			return msg, true
		}
	}

	return nil, false
}

func metadataFromValue(v any) map[string]string {
	switch meta := v.(type) {
	case *CommandEnvelope:
		return meta.Metadata
	case CommandEnvelope:
		return meta.Metadata
	case *EventEnvelope:
		return meta.Metadata
	case EventEnvelope:
		return meta.Metadata
	case wmmessage.Metadata:
		return map[string]string(meta)
	case map[string]string:
		return meta
	case *wmmessage.Message:
		return map[string]string(meta.Metadata)
	default:
		return map[string]string{}
	}
}

func inferKind(v any) MessageKind {
	meta := metadataFromValue(v)
	if kind := meta[MetadataMessageKind]; kind != "" {
		if strings.EqualFold(kind, string(KindEvent)) {
			return KindEvent
		}

		return KindCommand
	}

	return KindCommand
}

func normalizeSegment(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if versionSegment.MatchString(version) {
		return strings.ToLower(version)
	}

	if version == "" {
		return defaultVersion
	}

	return strings.ToLower(version)
}

func sanitizeTopic(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

func defaultServiceName() string {
	if svc := strings.TrimSpace(os.Getenv("SERVICE_NAME")); svc != "" {
		return strings.ToLower(svc)
	}

	return "shortlink"
}
