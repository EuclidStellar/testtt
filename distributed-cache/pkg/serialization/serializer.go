package serialization

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// SerializationFormat defines the serialization format
type SerializationFormat int

const (
	JSON SerializationFormat = iota
	GOB
	PROTOBUF
	MSGPACK
)

// Serializer interface for different serialization strategies
type Serializer interface {
	Serialize(data interface{}) ([]byte, error)
	Deserialize(data []byte, target interface{}) error
	ContentType() string
}

// JSONSerializer implements JSON serialization
type JSONSerializer struct{}

func (j *JSONSerializer) Serialize(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

func (j *JSONSerializer) Deserialize(data []byte, target interface{}) error {
	return json.Unmarshal(data, target)
}

func (j *JSONSerializer) ContentType() string {
	return "application/json"
}

// GobSerializer implements GOB serialization
type GobSerializer struct{}

func (g *GobSerializer) Serialize(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (g *GobSerializer) Deserialize(data []byte, target interface{}) error {
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	return decoder.Decode(target)
}

func (g *GobSerializer) ContentType() string {
	return "application/gob"
}

// ProtobufSerializer implements Protocol Buffers serialization
type ProtobufSerializer struct{}

func (p *ProtobufSerializer) Serialize(data interface{}) ([]byte, error) {
	if msg, ok := data.(proto.Message); ok {
		return proto.Marshal(msg)
	}
	return nil, fmt.Errorf("data is not a proto.Message")
}

func (p *ProtobufSerializer) Deserialize(data []byte, target interface{}) error {
	if msg, ok := target.(proto.Message); ok {
		return proto.Unmarshal(data, msg)
	}
	return fmt.Errorf("target is not a proto.Message")
}

func (p *ProtobufSerializer) ContentType() string {
	return "application/protobuf"
}

// SerializerFactory creates serializers based on format
type SerializerFactory struct{}

func (sf *SerializerFactory) CreateSerializer(format SerializationFormat) Serializer {
	switch format {
	case JSON:
		return &JSONSerializer{}
	case GOB:
		return &GobSerializer{}
	case PROTOBUF:
		return &ProtobufSerializer{}
	default:
		return &JSONSerializer{}
	}
}

// TypedValue wraps a value with its type information for generic serialization
type TypedValue struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// GenericSerializer handles arbitrary Go types
type GenericSerializer struct {
	serializer Serializer
}

func NewGenericSerializer(format SerializationFormat) *GenericSerializer {
	factory := &SerializerFactory{}
	return &GenericSerializer{
		serializer: factory.CreateSerializer(format),
	}
}

func (gs *GenericSerializer) SerializeTyped(data interface{}) ([]byte, error) {
	typedValue := TypedValue{
		Type:  reflect.TypeOf(data).String(),
		Value: data,
	}
	return gs.serializer.Serialize(typedValue)
}

func (gs *GenericSerializer) DeserializeTyped(data []byte) (interface{}, error) {
	var typedValue TypedValue
	if err := gs.serializer.Deserialize(data, &typedValue); err != nil {
		return nil, err
	}
	return typedValue.Value, nil
}

func (gs *GenericSerializer) Serialize(data interface{}) ([]byte, error) {
	return gs.serializer.Serialize(data)
}

func (gs *GenericSerializer) Deserialize(data []byte, target interface{}) error {
	return gs.serializer.Deserialize(data, target)
}

func (gs *GenericSerializer) ContentType() string {
	return gs.serializer.ContentType()
}

// CacheValue represents a serialized cache value with metadata
type CacheValue struct {
	Data         []byte              `json:"data"`
	ContentType  string              `json:"content_type"`
	TypeInfo     string              `json:"type_info,omitempty"`
	Compression  string              `json:"compression,omitempty"`
	Metadata     map[string]string   `json:"metadata,omitempty"`
}

// SerializationManager manages different serialization strategies
type SerializationManager struct {
	serializers map[SerializationFormat]Serializer
	defaultFormat SerializationFormat
}

func NewSerializationManager(defaultFormat SerializationFormat) *SerializationManager {
	factory := &SerializerFactory{}
	
	serializers := make(map[SerializationFormat]Serializer)
	serializers[JSON] = factory.CreateSerializer(JSON)
	serializers[GOB] = factory.CreateSerializer(GOB)
	serializers[PROTOBUF] = factory.CreateSerializer(PROTOBUF)
	
	return &SerializationManager{
		serializers:   serializers,
		defaultFormat: defaultFormat,
	}
}

func (sm *SerializationManager) SerializeValue(data interface{}, format SerializationFormat) (*CacheValue, error) {
	serializer, exists := sm.serializers[format]
	if !exists {
		serializer = sm.serializers[sm.defaultFormat]
		format = sm.defaultFormat
	}
	
	serializedData, err := serializer.Serialize(data)
	if err != nil {
		return nil, fmt.Errorf("serialization failed: %w", err)
	}
	
	return &CacheValue{
		Data:        serializedData,
		ContentType: serializer.ContentType(),
		TypeInfo:    reflect.TypeOf(data).String(),
		Metadata:    make(map[string]string),
	}, nil
}

func (sm *SerializationManager) DeserializeValue(cacheValue *CacheValue, target interface{}) error {
	// Determine format from content type
	var format SerializationFormat
	switch cacheValue.ContentType {
	case "application/json":
		format = JSON
	case "application/gob":
		format = GOB
	case "application/protobuf":
		format = PROTOBUF
	default:
		format = sm.defaultFormat
	}
	
	serializer, exists := sm.serializers[format]
	if !exists {
		return fmt.Errorf("unsupported serialization format: %s", cacheValue.ContentType)
	}
	
	return serializer.Deserialize(cacheValue.Data, target)
}

// Helper functions for common types
func SerializeString(value string, format SerializationFormat) ([]byte, error) {
	manager := NewSerializationManager(format)
	cacheValue, err := manager.SerializeValue(value, format)
	if err != nil {
		return nil, err
	}
	return cacheValue.Data, nil
}

func DeserializeString(data []byte, format SerializationFormat) (string, error) {
	manager := NewSerializationManager(format)
	cacheValue := &CacheValue{
		Data:        data,
		ContentType: manager.serializers[format].ContentType(),
	}
	
	var result string
	err := manager.DeserializeValue(cacheValue, &result)
	return result, err
}

func SerializeStruct(value interface{}, format SerializationFormat) ([]byte, error) {
	manager := NewSerializationManager(format)
	cacheValue, err := manager.SerializeValue(value, format)
	if err != nil {
		return nil, err
	}
	return cacheValue.Data, nil
}

func DeserializeStruct(data []byte, target interface{}, format SerializationFormat) error {
	manager := NewSerializationManager(format)
	cacheValue := &CacheValue{
		Data:        data,
		ContentType: manager.serializers[format].ContentType(),
	}
	
	return manager.DeserializeValue(cacheValue, target)
}
