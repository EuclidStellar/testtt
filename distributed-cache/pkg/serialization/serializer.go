package serialization

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
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

// MsgPackSerializer implements MessagePack serialization
type MsgPackSerializer struct{}

func (m *MsgPackSerializer) Serialize(data interface{}) ([]byte, error) {
	return msgpack.Marshal(data)
}

func (m *MsgPackSerializer) Deserialize(data []byte, target interface{}) error {
	return msgpack.Unmarshal(data, target)
}

func (m *MsgPackSerializer) ContentType() string {
	return "application/msgpack"
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
	case MSGPACK:
		return &MsgPackSerializer{}
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

// CompressionType defines compression algorithms
type CompressionType int

const (
	NoCompression CompressionType = iota
	GzipCompression
)

// EncryptionType defines encryption algorithms
type EncryptionType int

const (
	NoEncryption EncryptionType = iota
	AESEncryption
)

// AdvancedCacheValue represents a cache value with compression and encryption
type AdvancedCacheValue struct {
	Data         []byte                 `json:"data"`
	ContentType  string                 `json:"content_type"`
	TypeInfo     string                 `json:"type_info,omitempty"`
	Compression  CompressionType        `json:"compression"`
	Encryption   EncryptionType         `json:"encryption"`
	Metadata     map[string]string      `json:"metadata,omitempty"`
	Checksum     string                 `json:"checksum,omitempty"`
	Version      int                    `json:"version"`
}

// CompressorInterface defines compression operations
type CompressorInterface interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

// GzipCompressor implements gzip compression
type GzipCompressor struct{}

func (gc *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, fmt.Errorf("gzip compression failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (gc *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader creation failed: %w", err)
	}
	defer reader.Close()

	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip decompression failed: %w", err)
	}

	return result, nil
}

// EncryptorInterface defines encryption operations
type EncryptorInterface interface {
	Encrypt(data []byte, key []byte) ([]byte, error)
	Decrypt(data []byte, key []byte) ([]byte, error)
}

// AESEncryptor implements AES encryption
type AESEncryptor struct{}

func (ae *AESEncryptor) Encrypt(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation failed: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("GCM creation failed: %w", err)
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation failed: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func (ae *AESEncryptor) Decrypt(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("GCM creation failed: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("AES decryption failed: %w", err)
	}

	return plaintext, nil
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
	serializers   map[SerializationFormat]Serializer
	defaultFormat SerializationFormat
}

func NewSerializationManager(defaultFormat SerializationFormat) *SerializationManager {
	factory := &SerializerFactory{}

	serializers := make(map[SerializationFormat]Serializer)
	serializers[JSON] = factory.CreateSerializer(JSON)
	serializers[GOB] = factory.CreateSerializer(GOB)
	serializers[PROTOBUF] = factory.CreateSerializer(PROTOBUF)
	serializers[MSGPACK] = factory.CreateSerializer(MSGPACK)

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
	case "application/msgpack":
		format = MSGPACK
	default:
		format = sm.defaultFormat
	}

	serializer, exists := sm.serializers[format]
	if !exists {
		return fmt.Errorf("unsupported serialization format: %s", cacheValue.ContentType)
	}

	return serializer.Deserialize(cacheValue.Data, target)
}

// AdvancedSerializationManager manages serialization with compression and encryption
type AdvancedSerializationManager struct {
	*SerializationManager
	compressors map[CompressionType]CompressorInterface
	encryptors  map[EncryptionType]EncryptorInterface
	encryptionKey []byte
}

func NewAdvancedSerializationManager(defaultFormat SerializationFormat, encryptionKey []byte) *AdvancedSerializationManager {
	baseMgr := NewSerializationManager(defaultFormat)
	
	compressors := make(map[CompressionType]CompressorInterface)
	compressors[GzipCompression] = &GzipCompressor{}
	
	encryptors := make(map[EncryptionType]EncryptorInterface)
	encryptors[AESEncryption] = &AESEncryptor{}
	
	return &AdvancedSerializationManager{
		SerializationManager: baseMgr,
		compressors:         compressors,
		encryptors:          encryptors,
		encryptionKey:       encryptionKey,
	}
}

func (asm *AdvancedSerializationManager) SerializeAdvanced(data interface{}, format SerializationFormat, compression CompressionType, encryption EncryptionType) (*AdvancedCacheValue, error) {
	// First serialize the data
	cacheValue, err := asm.SerializeValue(data, format)
	if err != nil {
		return nil, fmt.Errorf("serialization failed: %w", err)
	}
	
	processedData := cacheValue.Data
	
	// Apply compression if requested
	if compression != NoCompression {
		compressor, exists := asm.compressors[compression]
		if !exists {
			return nil, fmt.Errorf("unsupported compression type: %d", compression)
		}
		
		processedData, err = compressor.Compress(processedData)
		if err != nil {
			return nil, fmt.Errorf("compression failed: %w", err)
		}
	}
	
	// Apply encryption if requested
	if encryption != NoEncryption && len(asm.encryptionKey) > 0 {
		encryptor, exists := asm.encryptors[encryption]
		if !exists {
			return nil, fmt.Errorf("unsupported encryption type: %d", encryption)
		}
		
		processedData, err = encryptor.Encrypt(processedData, asm.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
	}
	
	return &AdvancedCacheValue{
		Data:        processedData,
		ContentType: cacheValue.ContentType,
		TypeInfo:    cacheValue.TypeInfo,
		Compression: compression,
		Encryption:  encryption,
		Metadata:    cacheValue.Metadata,
		Version:     1,
	}, nil
}

func (asm *AdvancedSerializationManager) DeserializeAdvanced(advancedValue *AdvancedCacheValue, target interface{}) error {
	processedData := advancedValue.Data
	
	// Decrypt if needed
	if advancedValue.Encryption != NoEncryption && len(asm.encryptionKey) > 0 {
		encryptor, exists := asm.encryptors[advancedValue.Encryption]
		if !exists {
			return fmt.Errorf("unsupported encryption type: %d", advancedValue.Encryption)
		}
		
		decryptedData, err := encryptor.Decrypt(processedData, asm.encryptionKey)
		if err != nil {
			return fmt.Errorf("decryption failed: %w", err)
		}
		processedData = decryptedData
	}
	
	// Decompress if needed
	if advancedValue.Compression != NoCompression {
		compressor, exists := asm.compressors[advancedValue.Compression]
		if !exists {
			return fmt.Errorf("unsupported compression type: %d", advancedValue.Compression)
		}
		
		decompressedData, err := compressor.Decompress(processedData)
		if err != nil {
			return fmt.Errorf("decompression failed: %w", err)
		}
		processedData = decompressedData
	}
	
	// Create standard cache value for deserialization
	cacheValue := &CacheValue{
		Data:        processedData,
		ContentType: advancedValue.ContentType,
		TypeInfo:    advancedValue.TypeInfo,
		Metadata:    advancedValue.Metadata,
	}
	
	return asm.DeserializeValue(cacheValue, target)
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
