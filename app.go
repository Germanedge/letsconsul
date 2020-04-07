package main

import(
	"os"
	"log"
	"net"
	"time"
	"flag"
	"errors"
	"net/http"
	"strconv"
	"syscall"
	"os/signal"
	"fmt"

	"github.com/hashicorp/go-uuid"
	consul "github.com/hashicorp/consul/api"
)

type(
	App struct {
		Bind            string
		ConsulToken     string
		ConsulService   string        `consul:"service"`
		RenewInterval   time.Duration `consul:"renew_interval"`
		ReloadInterval  time.Duration `consul:"reload_interval"`
		consulClient    *consul.Client
		consulServiceID string
		letsconsul      *Letsconsul
	}
)

func (app *App) consulConfigure() error {
	kv := app.consulClient.KV()
	prefix := "letsconsul"

	kvPair, _, err := kv.Get(prefix + "/service", nil)
	if err != nil {
		return err
	}

	if kvPair == nil {
		fmt.Printf("no config found, provisioning consul\n")

                p := &consul.KVPair{Key: prefix + "/service", Value: []byte(prefix)}
	        _, err = kv.Put(p, nil)
	        if err != nil {
	                return err
	        }

		p = &consul.KVPair{Key: prefix + "/renew_interval", Value: []byte(os.Getenv("RENEW_INTERVAL"))}
		_, err = kv.Put(p, nil)
		if err != nil {
			return err
		}

		p = &consul.KVPair{Key: prefix + "/reload_interval", Value: []byte(os.Getenv("RELOAD_INTERVAL"))}
	        _, err = kv.Put(p, nil)
	        if err != nil {
	                return err
	        }

		p = &consul.KVPair{Key: prefix + "/domains_enabled", Value: []byte("[\"" + os.Getenv("DOMAINS_ENABLED") + "\"]")}
		_, err = kv.Put(p, nil)
		if err != nil {
			return err
		}

		p = &consul.KVPair{Key: prefix + "/domains/" + os.Getenv("DOMAINS_ENABLED") + "/domain_list" , Value: []byte("[\"" + os.Getenv("DOMAINS_ENABLED") + "\"]")}
	        _, err = kv.Put(p, nil)
	        if err != nil {
	                return err
	        }

		p = &consul.KVPair{Key: prefix + "/domains/" + os.Getenv("DOMAINS_ENABLED") + "/email" , Value: []byte(os.Getenv("EMAIL"))}
	        _, err = kv.Put(p, nil)
	        if err != nil {
	                return err
	        }

		p = &consul.KVPair{Key: prefix + "/domains/" + os.Getenv("DOMAINS_ENABLED") + "/timestamp", Value: []byte("0")}
	        _, err = kv.Put(p, nil)
	        if err != nil {
	                return err
	        }

		kvPair, _, err = kv.Get(prefix + "/service", nil)

	}

	app.ConsulService = string(kvPair.Value)

	kvPair_time, _, err := kv.Get(prefix + "/renew_interval", nil)
	if err != nil {
		        return err
	}

	app.RenewInterval, err = time.ParseDuration(string(kvPair_time.Value))
	if err != nil {
		return err
	}

	kvPair_interval, _, err := kv.Get(prefix + "/reload_interval", nil)
	if err != nil {
		return err
	}

	if kvPair == nil {
		return errors.New("Can't fetch 'reload_interval' key")
	}

	app.ReloadInterval, err = time.ParseDuration(string(kvPair_interval.Value))
	if err != nil {
		return err
	}

	return nil
}

func (app *App) config() error {
	app.ConsulToken = os.Getenv("CONSUL_TOKEN")

	bindPtr := flag.String("b", "0.0.0.0:8080", "host:port variable that validation web-server will listen")
	consulAddrPtr := flag.String("c", "127.0.0.1:8500", "consul address")
	flag.Parse()

	app.Bind = *bindPtr

	consulConfig := &consul.Config{
		Address:    *consulAddrPtr,
		Token:      app.ConsulToken,
		Scheme:     "http",
		HttpClient: http.DefaultClient,
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return err
	}

	app.consulClient = client

	err = app.consulConfigure()
	if err != nil {
		return err
	}

	go func() {
		for {
			time.Sleep(10 * time.Second)
			app.consulConfigure()
		}
	}()

	return nil
}

func (app *App) register() error {
	id, err := uuid.GenerateUUID()
	if err !=  nil {
		return err
	}

	app.consulServiceID = id

	checks := consul.AgentServiceChecks{
		&consul.AgentServiceCheck{
			TTL: "5s",
		},
	}

	_, portstr, err := net.SplitHostPort(app.Bind)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(portstr)
	if err != nil {
		return err
	}

	service := &consul.AgentServiceRegistration{
		ID: app.consulServiceID,
		Name: app.ConsulService,
		Port: port,
		Checks: checks,
	}

	agent := app.consulClient.Agent()
	err = agent.ServiceRegister(service)
	if err != nil {
		return err
	}

	go func() {
		for {
			<- time.After(2 * time.Second)
			err = agent.PassTTL("service:" + app.consulServiceID, "Internal TTL ping")
			if err != nil {
				log.Println(err)
			}
		}
	}()

	go func() {
		signalCh := make(chan os.Signal, 4)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
		<-signalCh
		log.Println("Interruption signal caught, exiting...")
		log.Println("Deregister service with ID:", app.consulServiceID)
		err := agent.ServiceDeregister(app.consulServiceID)
		if err != nil {
			log.Fatal("Deregistration failed. Reason:", err)
		} else {
			log.Println("Deregistration successful. Exiting with 0 exit code.")
			os.Exit(0)
		}
	}()

	time.Sleep(5 * time.Second)

	return nil
}

func (app *App) renewDomains() error {
	app.letsconsul.Domains = make(map[string]*DomainRecord)

	err := app.letsconsul.get(app.consulClient)
	if err != nil {
		return err
	}

	app.letsconsul.Bind = app.Bind

	err = app.letsconsul.renew(app.consulClient, app.RenewInterval)
	if err != nil {
		return err
	}

	return nil
}

func (app *App) start() {
	app.letsconsul = &Letsconsul{}

	for {
		err := app.renewDomains()
		if err != nil {
			log.Println(err)
		}

		<- time.After(app.ReloadInterval)
	}
}
