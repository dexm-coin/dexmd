package tests

// import (
// 	"crypto/rand"
// 	"testing"

// 	// "golang.org/x/crypto/sha3"
// 	"vuvuzela.io/crypto/bls"
// 	// bls "github.com/enzoh/go-bls"
// )

// func TestSignVerify(t *testing.T) {
// 	for i := 0; i < 100; i++ {
// 		msg := []byte("ciao")
// 		pub, priv, err := bls.GenerateKey(rand.Reader)
// 		if err != nil {
// 			t.Fatalf("error generating key: %s", err)
// 		}
// 		pub2, priv2, err := bls.GenerateKey(rand.Reader)
// 		if err != nil {
// 			t.Fatalf("error generating key: %s", err)
// 		}
// 		pub3, priv3, err := bls.GenerateKey(rand.Reader)
// 		if err != nil {
// 			t.Fatalf("error generating key: %s", err)
// 		}

// 		sig := bls.Sign(priv, msg)
// 		sig2 := bls.Sign(priv2, msg)
// 		sig3 := bls.Sign(priv3, msg)
// 		ok := bls.Verify([]*bls.PublicKey{pub, pub2, pub3}, [][]byte{msg}, sig)
// 		if !ok {
// 			t.Fatal("failed")
// 			// t.Fatalf("expected signature to verify: msg=%q pub=%s priv=%s sig=%#v", msg, pub.gx, priv.x, sig)
// 		}
// 		fmt.Println(ok)
// 		ok = bls.Verify([]*bls.PublicKey{pub2, pub, pub3}, [][]byte{msg}, sig2)
// 		if !ok {
// 			t.Fatal("failed")
// 		}
// 		fmt.Println(ok)
// 		ok = bls.Verify([]*bls.PublicKey{pub2}, [][]byte{msg}, sig3)
// 		if !ok {
// 			t.Fatal("failed")
// 		}
// 		fmt.Println(ok)

// 		// compressedSig := sig.Compress()
// 		// ok = bls.VerifyCompressed([]*bls.PublicKey{pub}, [][]byte{msg}, compressedSig)
// 		// if !ok {
// 		// 	t.Fatal("failed")
// 		// 	// t.Fatalf("expected compressed signature to verify: msg=%q pub=%s priv=%s sig=%#v", msg, pub.gx, priv.x, sig)
// 		// }

// 		// msg[0] = ^msg[0]
// 		// ok = bls.Verify([]*bls.PublicKey{pub}, [][]byte{msg}, sig)
// 		// if ok {
// 		// 	t.Fatalf("expected signature to not verify")
// 		// }
// 		fmt.Println(ok)
// 	}
// }

// func TestAggregate(t *testing.T) {
// 	msg1 := randomMessage()
// 	msg2 := randomMessage()
// 	msg3 := randomMessage()

// 	pub1, priv1, _ := bls.GenerateKey(rand.Reader)
// 	pub2, priv2, _ := bls.GenerateKey(rand.Reader)
// 	pub3, priv3, _ := bls.GenerateKey(rand.Reader)

// 	sig1 := bls.Sign(priv1, msg1)
// 	sig2 := bls.Sign(priv2, msg2)
// 	sig3 := bls.Sign(priv3, msg3)

// 	sig := bls.Aggregate(sig1, sig2, sig3)

// 	ok := bls.Verify([]*bls.PublicKey{pub1, pub2, pub3}, [][]byte{msg1, msg2, msg3}, sig)
// 	if !ok {
// 		t.Fatalf("failed to verify aggregate signature")
// 	}

// 	shortSig := sig.Compress()
// 	ok = bls.VerifyCompressed([]*bls.PublicKey{pub1, pub2, pub3}, [][]byte{msg1, msg2, msg3}, shortSig)
// 	if !ok {
// 		t.Fatalf("failed to verify compressed aggregate signature")
// 	}
// }

// func randomMessage() []byte {
// 	msg := make([]byte, 1000000)
// 	rand.Read(msg)
// 	return msg
// }

// func TestKnownAnswer(t *testing.T) {
// 	msg := []byte("test message")
// 	// We use sha3 instead of a zeroReader because GenerateKey
// 	// loops forever with a zeroReader.
// 	_, priv, err := GenerateKey(sha3.NewShake128())
// 	if err != nil {
// 		t.Fatalf("error generating key: %s", err)
// 	}

// 	actual := Sign(priv, msg)
// 	expected, _ := decodeText([]byte("cAdm38gylnKd5Tq43SscV0K2ShgZ-f3-Acc-1WTV84U4QTwqCcrV47vxVl27yq2v5VGrxNJTB97V1CuQnprH1w"))
// 	if !bytes.Equal(actual, expected) {
// 		t.Fatalf("got %s, want %s", actual, expected)
// 	}
// }

// import (
// 	"crypto/sha256"
// 	"testing"

// 	bls "github.com/enzoh/go-bls"
// )

// func TestAggregateVerify(test *testing.T) {

// 	messages := "This is a message."
// 	n := 10

// 	// Generate key pairs.
// 	params, err := bls.GenParamsTypeD(9563, 512)
// 	if err != nil {
// 		test.Fatal(err)
// 	}
// 	pairing := bls.GenPairing(params)
// 	system, err := bls.GenSystem(pairing)
// 	if err != nil {
// 		test.Fatal(err)
// 	}
// 	keys := make([]bls.PublicKey, n)
// 	secrets := make([]bls.PrivateKey, n)
// 	for i := 0; i < n; i++ {
// 		keys[i], secrets[i], err = bls.GenKeys(system)
// 		if err != nil {
// 			test.Fatal(err)
// 		}
// 	}

// 	// Sign the messages.
// 	hashes := make([][sha256.Size]byte, n)
// 	signatures := make([]bls.Signature, n)
// 	for i := 0; i < n; i++ {
// 		hashes[i] = sha256.Sum256([]byte(messages))
// 		signatures[i] = bls.Sign(hashes[i], secrets[i])
// 	}

// 	// Aggregate the signatures.
// 	aggregate, err := bls.Aggregate(signatures, system)
// 	if err != nil {
// 		test.Fatal(err)
// 	}

// 	// Verify the aggregate signature.
// 	valid, err := bls.AggregateVerify(aggregate, hashes, keys)
// 	if err != nil {
// 		test.Fatal(err)
// 	}
// 	if !valid {
// 		test.Fatal("Failed to verify aggregate signature.")
// 	}

// 	// Clean up.
// 	aggregate.Free()
// 	for i := 0; i < n; i++ {
// 		signatures[i].Free()
// 		keys[i].Free()
// 		secrets[i].Free()
// 	}
// 	system.Free()
// 	pairing.Free()
// 	params.Free()

// }
