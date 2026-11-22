ALTER TABLE mcp_connection ADD COLUMN permission TEXT NOT NULL DEFAULT 'read_write' CHECK (permission IN ('read_only', 'read_write'));
