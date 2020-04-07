package main

import(
	"fmt"
	"time"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/ecdsa"
	"errors"
	"strconv"
	"encoding/json"


	"github.com/go-acme/lego/certcrypto"
	"github.com/go-acme/lego/certificate"
	"github.com/go-acme/lego/challenge/http01"
	//"github.com/go-acme/lego/challenge/tlsalpn01"
	"github.com/go-acme/lego/lego"
	"github.com/go-acme/lego/registration"
	consul "github.com/hashicorp/consul/api"
)

type(
	DomainRecord struct {
		Domains    []string  `consul:"domains"`
		Email      string    `consul:"email"`
		Timestamp  time.Time `consul:"timestamp"`
		PrivateKey string    `consul:"private_key"`
		Fullchain  string    `consul:"fullchain"`
		key   *ecdsa.PrivateKey
		Reg   *registration.Resource
	}
)

type MyUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}
func (u *MyUser) GetEmail() string {
	return u.Email
}
func (u MyUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *MyUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}


func (domainRecord *DomainRecord) GetEmail() string { return domainRecord.Email }
func (domainRecord *DomainRecord) GetRegistration() *registration.Resource { return domainRecord.Reg }
func (domainRecord *DomainRecord) GetPrivateKey() crypto.PrivateKey { return domainRecord.key }

func (domainRecord *DomainRecord) write(client *consul.Client, domainRecordName string) error {
	kv := client.KV()
	consulService := "letsconsul"

	timestamp := domainRecord.Timestamp.Unix()
	timestampStr := strconv.Itoa(int(timestamp))

	p := &consul.KVPair {
		Key: consulService + "/domains/" + domainRecordName + "/timestamp",
		Value: []byte(timestampStr),
	}
	_, err := kv.Put(p, nil)
	if err != nil {
		return err
	}

	p = &consul.KVPair {
		Key: consulService + "/domains/" + domainRecordName + "/fullchain",
		Value: []byte(domainRecord.Fullchain),
	}
	_, err = kv.Put(p, nil)
	if err != nil {
		return err
	}

	p = &consul.KVPair {
		Key: consulService + "/domains/" + domainRecordName + "/private_key",
		Value: []byte(domainRecord.PrivateKey),
	}
	_, err = kv.Put(p, nil)
	if err != nil {
		return err
	}

	return nil
}

func kvFetch(kv *consul.KV, prefix string, domainRecordName string, key string) ([]byte, error) {
	kvPair, _, err := kv.Get(prefix + "/domains/" + domainRecordName + "/" + key, nil)
	if err != nil {
		return nil, err
	}

	if kvPair == nil {
		return nil, errors.New("Can't fetch '" + key + "' key from '" + domainRecordName + "' domain")
	}

	return kvPair.Value, nil
}

func (domainRecord *DomainRecord) get(kv *consul.KV, prefix string, domainRecordName string) error {
	v, err := kvFetch(kv, prefix, domainRecordName, "domain_list")
	if err != nil {
		return err
	}

	err = json.Unmarshal(v, &domainRecord.Domains)
	if err != nil {
		return err
	}

	v, err = kvFetch(kv, prefix, domainRecordName, "email")
	if err != nil {
		return err
	}

	domainRecord.Email = string(v)

	v, err = kvFetch(kv, prefix, domainRecordName, "timestamp")
	if err != nil {
		return err
	}

	i, err := strconv.ParseInt(string(v), 10, 64)
	if err != nil {
		return err
	}

	domainRecord.Timestamp = time.Unix(i, 0)

	return nil
}

func (domainRecord *DomainRecord) renew(bind string) error {
	// Create a user. New accounts need an email and private key to start.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	domainRecord.key = privateKey

	myUser := MyUser{
		Email: "christoph.heuwieser@germanedge.com",
		key:   privateKey,
	}

	config := lego.NewConfig(&myUser)

	// This CA URL is configured for a local dev instance of Boulder running in Docker in a VM.
	//config.CADirURL = "http://192.168.99.100:4000/directory"
	config.Certificate.KeyType = certcrypto.RSA2048

	// A client facilitates communication with the CA server.
	client, err := lego.NewClient(config)
	if err != nil {
		return err
	}

	// We specify an http port of 5002 and an tls port of 5001 on all interfaces
	// because we aren't running as root and can't bind a listener to port 80 and 443
	// (used later when we attempt to pass challenges). Keep in mind that you still
	// need to proxy challenge traffic to port 5002 and 5001.
	err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "5002"))
	if err != nil {
		return err
	}
	//err = client.Challenge.SetTLSALPN01Provider(tlsalpn01.NewProviderServer("", "5001"))
	//if err != nil {
	//	return err
	//}

	// New users will need to register
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return err
	}
	myUser.Registration = reg
	domainRecord.Reg = reg

	request := certificate.ObtainRequest{
		Domains: domainRecord.Domains,
		Bundle:  true,
	}
	acmeCert, err := client.Certificate.Obtain(request)
	if err != nil {
		return err
	}

	domainRecord.Fullchain = string(acmeCert.Certificate)
	domainRecord.PrivateKey = string(acmeCert.PrivateKey)
	domainRecord.Timestamp = time.Now()

	// Each certificate comes back with the cert bytes, the bytes of the client's
	// private key, and a certificate URL. SAVE THESE TO DISK.
	fmt.Printf("%#v\n", acmeCert)

	return nil
}
