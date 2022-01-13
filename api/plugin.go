package api

import (
	"bufio"
	"fmt"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"io"
	"os/exec"
	"strings"
)

type Plugin struct {
	FullName       string `json:"name"`       // FullName of the plugin, must be a go `get`-able package
	ExecutableName string `json:"executable"` // Name of the executable installed by go install
	client         *grpc.ClientConn
	cmd            *exec.Cmd
	ready          bool
}

func (p *Plugin) Init() error {
	packageName := p.FullName
	if !strings.Contains(packageName, "@") {
		packageName = packageName + "@latest"
	}
	cmd := exec.Command("go", "install", packageName)
	err := cmd.Run()
	if err != nil {
		b, _ := cmd.CombinedOutput()
		log.Errorf("Failed to install plugin %s with %+v\n%s", p.FullName, err, string(b))
	}
	p.ready = true
	return err
}

func (p *Plugin) IsReady() bool {
	return p.ready
}

func (p *Plugin) IsConnected() bool {
	return p.client != nil && p.cmd != nil
}

func (p *Plugin) GetConnection() grpc.ClientConnInterface {
	return p.client
}

func (p *Plugin) Start(args ...string) error {
	p.cmd = exec.Command(p.ExecutableName, args...)

	reader, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = p.cmd.Start()
	if err != nil {
		log.Errorf("Failed to start plugin %s", p.FullName)
	}

	err = p.Interact(reader)
	return err
}

func (p *Plugin) Interact(in io.Reader) error {
	reader := bufio.NewScanner(in)
	for reader.Scan() {
		line := reader.Text()

		if reader.Err() != nil {
			log.Errorf("Failed to read plugin %s", p.FullName)
		}

		addr := line

		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			log.Errorf("[%s] Failed to connect to plugin at %s : %+v", p.FullName, addr, err)
		}

		p.client = conn
		return err
	}
	return fmt.Errorf("failed to interact with process")
}

func (p *Plugin) Stop() {
	if p.client != nil {
		_ = p.client.Close()
	}

	if p.cmd != nil {
		_ = p.cmd.Process.Kill()
	}
}
