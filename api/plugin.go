package api

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"os/exec"
)

type Plugin struct {
	FullName       string `json:"name"`       // FullName of the plugin, must be a go `get`-able package
	ExecutableName string `json:"executable"` // Name of the executable installed by go install
	client         *grpc.ClientConn
	cmd            *exec.Cmd
}

func (p *Plugin) Init() error {
	cmd := exec.Command("go", "-u", "install", p.FullName)
	err := cmd.Run()
	if err != nil {
		log.Errorf("Failed to install plugin %s", p.FullName)
	}

	return err
}

func (p *Plugin) IsConnected() bool {
	return p.client != nil && p.cmd != nil
}

func (p *Plugin) GetConnection() grpc.ClientConnInterface {
	return p.client
}

func (p *Plugin) Start() error {
	p.cmd = exec.Command(p.ExecutableName)

	reader := new(bytes.Buffer)
	p.cmd.Stdout = reader

	go func() {
		err := p.cmd.Run()
		if err != nil {
			log.Errorf("Failed to start plugin %s", p.FullName)
		}
	}()

	line, err := reader.ReadBytes('\n')

	if err != nil {
		log.Errorf("Failed to read plugin %s", p.FullName)
	}

	addr := string(line)

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Errorf("[%s] Failed to connect to plugin at %s : %+v", p.FullName, addr, err)
	}

	p.client = conn
	return err
}

func (p *Plugin) Stop() {
	if p.client != nil {
		_ = p.client.Close()
	}

	if p.cmd != nil {
		_ = p.cmd.Process.Kill()
	}
}
