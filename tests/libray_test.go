package tests

import (
	"bytes"
	"encoding/gob"
	"fmt"
	// "reflect"
	"testing"

	"gopkg.in/dedis/kyber.v2"
	"gopkg.in/mgo.v2/bson"
)

type Person struct {
	Name  string
	Phone string
}

func TestBson(t *testing.T) {
	data, err := bson.Marshal(&Person{Name: "Bob"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q", data)

	var q Person
	bson.Unmarshal(data, &q)
	fmt.Printf("%q", q)
}

func Marshal(v interface{}) ([]byte, error) {
	b := new(bytes.Buffer)
	err := gob.NewEncoder(b).Encode(v)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func Unmarshal(data []byte, v interface{}) error {
	b := bytes.NewBuffer(data)
	return gob.NewDecoder(b).Decode(v)
}

type Payload struct {
	Int    int
	String string
	List   []int
}

func MarshalGob(v interface{}) ([]byte, error) {
	b := new(bytes.Buffer)
	err := gob.NewEncoder(b).Encode(v)
	if err != nil {
		fmt.Println("ERROR ", err)
		return nil, err
	}
	return b.Bytes(), nil
}

func UnmarshalGob(data []byte, v *kyber.Point) error {
	b := bytes.NewBuffer(data)
	return gob.NewDecoder(b).Decode(v)
}

func SignatureToByte(S Signature) ([]byte, error) {
	sByte, err := MarshalGob(S)
	return sByte, err
}

func ByteToPoint(b []byte, p kyber.Point) {
	fmt.Println("before Q ", p)
	err := UnmarshalGob(b, &p)
	fmt.Println("ERROR ", err)
	fmt.Println("after Q ", p)
}

type Incer interface {
	gob.GobEncoder
	gob.GobDecoder
	Inc()
}

func TestGob2(t *testing.T) {
	k := curve.Scalar().Pick(curve.RandomStream())
	r := curve.Point().Mul(k, g)
	fmt.Println(r)
	// fmt.Print(reflect.TypeOf(r))

	res, err := r.MarshalBinary()
	fmt.Println("err ", err)
	fmt.Println(res)

	var q kyber.Point
	q = curve.Point()
	err = q.UnmarshalBinary(res)
	fmt.Println("err ", err)
	fmt.Println(q)



	// b := new(bytes.Buffer)
	// err := gob.NewEncoder(b).Encode(r)
	// fmt.Println("err ", err)
	// rByte := b.Bytes()
	// fmt.Println("rByte ", rByte)

	// var q kyber.Point
	// q = curve.Point()

	// b2 := bytes.NewBuffer(rByte)
	// err = gob.NewDecoder(b2).Decode(q)
	// fmt.Println("q ", q)
}

func TestGob(t *testing.T) {
	data := Payload{42, "Hello, World", []int{3, 4, 5}}
	fmt.Printf("before %+v\n", data)

	encoded, err := Marshal(data)
	if err != nil {
		t.Error(err)
	}

	var decoded Payload
	if err := Unmarshal(encoded, &decoded); err != nil {
		t.Error(err)
	}

	fmt.Printf("after %+v\n", decoded)
}
