package dwgd

import (
	"net"

	"github.com/docker/go-plugins-helpers/network"
)

type Dwgd struct {
	driver    *Driver
	handler   *network.Handler
	listener  net.Listener
	symlinker *RootlessSymlinker
}

func NewDwgd(cfg *Config) (*Dwgd, error) {
	driver, err := NewDriver(cfg.Db, nil, nil)
	if err != nil {
		return nil, err
	}

	handler := network.NewHandler(driver)

	listener, err := NewUnixListener(nil)
	if err != nil {
		return nil, err
	}

	var symlinker *RootlessSymlinker
	if cfg.Rootless {
		symlinker, err = NewRootlessSymlinker(nil)
		if err != nil {
			return nil, err
		}
	}

	return &Dwgd{
		driver:    driver,
		handler:   handler,
		listener:  listener,
		symlinker: symlinker,
	}, nil
}

func (d *Dwgd) Start() error {
	go func() {
		err := d.handler.Serve(d.listener)
		if err != nil {
			TraceLog.Printf("Couldn't serve on unix socket: %s\n", err)
		}
	}()

	if d.symlinker != nil {
		go func() {
			err := d.symlinker.Start()
			if err != nil {
				TraceLog.Printf("Couldn't start symlinker: %s\n", err)
			}
		}()
	}

	return nil
}

func (d *Dwgd) Stop() error {
	TraceLog.Println("Closing driver")
	err := d.driver.Close()
	if err != nil {
		TraceLog.Printf("Error during driver close: %s\n", err)
	}

	TraceLog.Println("Closing listener")
	err = d.listener.Close()
	if err != nil {
		TraceLog.Printf("Error during listener close: %s\n", err)
	}

	if d.symlinker != nil {
		TraceLog.Println("Closing symlinker")
		err := d.symlinker.Stop()
		if err != nil {
			TraceLog.Printf("Error during symlinker close: %s\n", err)
		}
	} else {
		TraceLog.Println("Symlinker not set, skipping closing")
	}

	return nil
}
