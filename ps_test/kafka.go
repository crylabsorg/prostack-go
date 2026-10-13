package ps_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func NewKafkaContainer(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	// Discover the mapped port before starting Kafka so metadata advertises the
	// same address clients use to reach the container.
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "apache/kafka:4.1.2", ExposedPorts: []string{"9092/tcp"},
			Entrypoint: []string{"/bin/sh", "-c"},
			Cmd:        []string{"while [ ! -f /tmp/start-kafka.sh ]; do sleep 0.1; done; exec /bin/sh /tmp/start-kafka.sh"},
		},
		Started: true,
	})
	if container != nil {
		t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	}
	require.NoError(t, err)
	endpoint, err := container.PortEndpoint(ctx, "9092/tcp", "")
	require.NoError(t, err)
	script := fmt.Sprintf("sed -i 's|^advertised.listeners=.*|advertised.listeners=PLAINTEXT://%s|' /etc/kafka/docker/server.properties\nexec /etc/kafka/docker/run\n", endpoint)
	require.NoError(t, container.CopyToContainer(ctx, []byte(script), "/tmp/start-kafka.sh", 0o755))
	if err := wait.ForLog("Kafka Server started").WithStartupTimeout(2*time.Minute).WaitUntilReady(ctx, container); err != nil {
		logs, logErr := container.Logs(ctx)
		require.NoError(t, logErr)
		defer func() { require.NoError(t, logs.Close()) }()
		output, readErr := io.ReadAll(logs)
		require.NoError(t, readErr)
		t.Log(string(output))
		require.NoError(t, err)
	}
	return []string{endpoint}
}
