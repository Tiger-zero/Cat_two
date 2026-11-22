ALTER TABLE mcp_connection ADD COLUMN permission VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'read_write' CHECK (permission IN ('read_only', 'read_write'));
