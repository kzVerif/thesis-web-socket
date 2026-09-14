package repository

import "context"

func (r *AgentRepository) WriteAudit(ctx context.Context, user, action, agent, ip string, detail []byte) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO logs(user_id,action,target_agent_id,ip_address,detail)
	VALUES(NULLIF($1,'')::uuid,$2,(SELECT id FROM agents WHERE id::text=$3),NULLIF($4,'')::inet,$5::jsonb)`, user, action, agent, ip, string(detail))
	return err
}
