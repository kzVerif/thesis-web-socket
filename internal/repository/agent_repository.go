package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"ws-rat/internal/model"
)

type AgentRepository struct {
	db *sql.DB
}

func NewAgentRepository(db *sql.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

func (repository *AgentRepository) GetByID(ctx context.Context, id string) (*model.AgentInfo, error) {
	var agent model.AgentInfo
	var osInfoJSON []byte
	err := repository.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(room_id::text, ''), hostname, os_info, ip_address::text, mac_address::text
		FROM agents WHERE id = $1`, id,
	).Scan(&agent.ID, &agent.RoomID, &agent.Hostname, &osInfoJSON, &agent.IPAddress, &agent.MACAddress)
	if err != nil {
		return nil, fmt.Errorf("get agent %s: %w", id, err)
	}
	if err := json.Unmarshal(osInfoJSON, &agent.OSInfo); err != nil {
		return nil, fmt.Errorf("decode agent %s os_info: %w", id, err)
	}
	return &agent, nil
}

func (repository *AgentRepository) UpdateStatus(ctx context.Context, id, status string) error {
	if status != model.StatusOnline && status != model.StatusOffline {
		return fmt.Errorf("invalid agent status %q", status)
	}

	result, err := repository.db.ExecContext(ctx, statusUpdateQuery(status), status, id)
	if err != nil {
		return fmt.Errorf("update agent %s status: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("agent %s does not exist", id)
	}
	return nil
}

func statusUpdateQuery(status string) string {
	if status == model.StatusOnline {
		return "UPDATE agents SET status = $1, last_seen = NOW() WHERE id = $2"
	}
	return "UPDATE agents SET status = $1 WHERE id = $2"
}
