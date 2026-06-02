package nginx

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Controller drives the separate nginx container via the Docker SDK.
type Controller struct {
	cli      *client.Client
	name     string
	mainConf string // path nginx runs with (nginx -t -c)
	dryRun   bool
}

// NewController builds a controller. dry-run (a no-op) is used ONLY when
// explicitly requested via PANEL_NGINX_DRYRUN; otherwise a Docker client that
// cannot be created is a hard error (fail-fast) rather than a silent no-op that
// would make applies appear to succeed without validating/reloading nginx.
func NewController(dockerHost, containerName, mainConf string, dryRun bool) (*Controller, error) {
	if dryRun {
		log.Printf("[nginx] dry-run mode: nginx -t / reload will be skipped (PANEL_NGINX_DRYRUN=true)")
		return &Controller{name: containerName, mainConf: mainConf, dryRun: true}, nil
	}
	cli, err := client.NewClientWithOpts(client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client 初始化失败（本地无 Docker 请设置 PANEL_NGINX_DRYRUN=true）: %w", err)
	}
	return &Controller{cli: cli, name: containerName, mainConf: mainConf}, nil
}

// DryRun reports whether nginx control is a no-op.
func (c *Controller) DryRun() bool { return c.dryRun }

func (c *Controller) execCapture(ctx context.Context, cmd []string) (stdout, stderr string, exitCode int, err error) {
	cr, err := c.cli.ContainerExecCreate(ctx, c.name, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", "", -1, fmt.Errorf("exec create: %w", err)
	}
	att, err := c.cli.ContainerExecAttach(ctx, cr.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", -1, fmt.Errorf("exec attach: %w", err)
	}
	defer att.Close()

	var outBuf, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&outBuf, &errBuf, att.Reader); err != nil {
		return "", "", -1, fmt.Errorf("read exec stream: %w", err)
	}
	insp, err := c.cli.ContainerExecInspect(ctx, cr.ID)
	if err != nil {
		return outBuf.String(), errBuf.String(), -1, fmt.Errorf("exec inspect: %w", err)
	}
	return outBuf.String(), errBuf.String(), insp.ExitCode, nil
}

// TestConfig runs `nginx -t`. ok is true when the config is valid. output is the
// combined stdout+stderr (nginx writes test results to stderr).
func (c *Controller) TestConfig(ctx context.Context) (output string, ok bool, err error) {
	if c.dryRun {
		return "dry-run: skipped nginx -t", true, nil
	}
	cmd := []string{"nginx", "-t"}
	if c.mainConf != "" {
		cmd = append(cmd, "-c", c.mainConf)
	}
	out, errOut, code, err := c.execCapture(ctx, cmd)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(out + errOut), code == 0, nil
}

// Reload runs `nginx -s reload`.
func (c *Controller) Reload(ctx context.Context) error {
	if c.dryRun {
		return nil
	}
	out, errOut, code, err := c.execCapture(ctx, []string{"nginx", "-s", "reload"})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("nginx reload failed: %s", strings.TrimSpace(out+errOut))
	}
	return nil
}
