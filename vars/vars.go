package vars

var CommonSecret = []byte("HelloThereCamServer")

const (
	EncryptCost      = 17
	HandshakeCost    = 16
	HandshakeLen     = 128
	HandshakeHashLen = 256

	TouchPasswordLen = 10
)
