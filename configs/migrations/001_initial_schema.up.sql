-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create messages table with partitioning
CREATE TABLE IF NOT EXISTS messages (
    id SERIAL,
    tenant_id INTEGER NOT NULL,
    queue_name VARCHAR(255) NOT NULL,
    message_body TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE,
    PRIMARY KEY (id, tenant_id)
) PARTITION BY HASH (tenant_id);

-- Create partitions for tenant_id 1-10
CREATE TABLE IF NOT EXISTS messages_0 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 0);
CREATE TABLE IF NOT EXISTS messages_1 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 1);
CREATE TABLE IF NOT EXISTS messages_2 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 2);
CREATE TABLE IF NOT EXISTS messages_3 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 3);
CREATE TABLE IF NOT EXISTS messages_4 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 4);
CREATE TABLE IF NOT EXISTS messages_5 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 5);
CREATE TABLE IF NOT EXISTS messages_6 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 6);
CREATE TABLE IF NOT EXISTS messages_7 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 7);
CREATE TABLE IF NOT EXISTS messages_8 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 8);
CREATE TABLE IF NOT EXISTS messages_9 PARTITION OF messages FOR VALUES WITH (modulus 10, remainder 9);

-- Create consumer configurations table
CREATE TABLE IF NOT EXISTS consumer_configs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    queue_name VARCHAR(255) NOT NULL,
    workers INTEGER NOT NULL DEFAULT 3,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, queue_name)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_messages_tenant_id ON messages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_consumer_configs_tenant_queue ON consumer_configs(tenant_id, queue_name);

-- Insert default tenant
INSERT INTO tenants (id, name) VALUES (1, 'default') ON CONFLICT (id) DO NOTHING;

-- Insert default consumer config
INSERT INTO consumer_configs (tenant_id, queue_name, workers) VALUES (1, 'tenant_1_queue', 3) ON CONFLICT (tenant_id, queue_name) DO NOTHING; 