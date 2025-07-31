package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"time"
)

type Cipher struct {
	key   []byte
	block cipher.Block
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &Cipher{
		key:   key,
		block: block,
	}, nil
}

func (c *Cipher) Encrypt(packet []byte) ([]byte, error) {
	ciphertext := make([]byte, aes.BlockSize+len(packet))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(c.block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], packet)
	return ciphertext, nil
}

func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	text := make([]byte, len(ciphertext))
	stream := cipher.NewCTR(c.block, iv)
	stream.XORKeyStream(text, ciphertext)
	return text, nil
}

func GenerateCertificate() (tls.Certificate, error) {
	privacy, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"VPN Server"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(0, 0, 0, 0)},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privacy.PublicKey, privacy)
	if err != nil {
		return tls.Certificate{}, err
	}
	cerPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privacy)})
	return tls.X509KeyPair(cerPEM, keyPEM)
}

func NewServerTSLConfig() (*tls.Config, error) {
	cert, err := GenerateCertificate()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func NewClientTSLConfig(skipVerify bool) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: skipVerify,
		MinVersion:         tls.VersionTLS12,
	}
}
