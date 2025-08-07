-- Drop indexes
DROP INDEX IF EXISTS idx_messages_tenant_id;
DROP INDEX IF EXISTS idx_messages_status;
DROP INDEX IF EXISTS idx_messages_created_at;
DROP INDEX IF EXISTS idx_consumer_configs_tenant_queue;

-- Drop tables
DROP TABLE IF EXISTS consumer_configs;
DROP TABLE IF EXISTS messages_0;
DROP TABLE IF EXISTS messages_1;
DROP TABLE IF EXISTS messages_2;
DROP TABLE IF EXISTS messages_3;
DROP TABLE IF EXISTS messages_4;
DROP TABLE IF EXISTS messages_5;
DROP TABLE IF EXISTS messages_6;
DROP TABLE IF EXISTS messages_7;
DROP TABLE IF EXISTS messages_8;
DROP TABLE IF EXISTS messages_9;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS tenants; 