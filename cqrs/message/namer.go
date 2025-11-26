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
)

var (
	versionSegment = regexp.MustCompile(`^v[0-9]+$`)
)

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

func buildNameComponents(v any, fallbackService, fallbackKind, fallbackVersion string) nameComponents {
	comps := nameComponents{
		Service: fallbackService,
		Kind:    fallbackKind,
		Name:    "",
		Version: fallbackVersion,
	}

	meta := metadataFromValue(v)
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
		if msg, ok := toProto(v); ok {
			full := string(proto.MessageName(msg))
			assignComponentsFromProto(&comps, full)
		}
	}

	if comps.Name == "" {
		comps.Name = camelToSnake(typeNameOf(v))
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

func assignComponentsFromProto(c *nameComponents, full string) {
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
		c.Name = camelToSnake(typeName)
		
		// For events, extract aggregate from protobuf package if Kind is still "event"
		// Format: domain.{aggregate}.v1.TypeName -> aggregate = parts[1]
		if c.Kind == string(KindEvent) && len(parts) >= 3 {
			// Extract aggregate from protobuf package (second segment)
			aggregate := normalizeSegment(parts[1])
			if aggregate != "" {
				c.Kind = aggregate
			}
			// Remove aggregate prefix from event name if present
			// e.g., "LinkCreated" -> "created" (if aggregate is "link")
			eventName := camelToSnake(typeName)
			if strings.HasPrefix(strings.ToLower(eventName), strings.ToLower(aggregate)+"_") {
				c.Name = strings.TrimPrefix(eventName, strings.ToLower(aggregate)+"_")
			}
		}
	}
	
	// Only extract version if it's not already set and protobuf package has version segment
	if c.Version == "" && len(parts) >= 3 && versionSegment.MatchString(parts[len(parts)-2]) {
		c.Version = parts[len(parts)-2]
	}
}

func assignComponentsFromQualifiedName(c *nameComponents, qualified string) {
	segments := strings.Split(qualified, ".")
	switch len(segments) {
	case 0:
		return
	case 1:
		c.Name = segments[0]
	case 2:
		c.Kind = segments[0]
		c.Name = segments[1]
	default:
		c.Service = segments[0]
		c.Kind = segments[1]
		c.Name = segments[len(segments)-1]
	}
}

func camelToSnake(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func typeNameOf(v any) string {
	if v == nil {
		return ""
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		if t.Elem() == nil {
			break
		}
		t = t.Elem()
	}
	return t.Name()
}

func toProto(v any) (proto.Message, bool) {
	if v == nil {
		return nil, false
	}
	if msg, ok := v.(proto.Message); ok {
		return msg, true
	}
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return nil, false
	}
	if val.Kind() == reflect.Pointer && val.IsNil() {
		// Create zero instance of the pointer element.
		elem := reflect.New(val.Type().Elem())
		if msg, ok := elem.Interface().(proto.Message); ok {
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

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if versionSegment.MatchString(v) {
		return strings.ToLower(v)
	}
	if v == "" {
		return defaultVersion
	}
	return strings.ToLower(v)
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
