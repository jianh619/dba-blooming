#!/bin/bash
set -euo pipefail

# Validate all required environment variables before generating any config.
# Unset variables expand silently to empty strings without this guard,
# which would produce a Patroni config with empty passwords.
required_vars=(
  PATRONI_SCOPE
  PATRONI_NAME
  PATRONI_RESTAPI_CONNECT_ADDRESS
  PATRONI_ETCD3_HOSTS
  PATRONI_REPLICATION_PASSWORD
  PATRONI_SUPERUSER_PASSWORD
)
for var in "${required_vars[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: required environment variable '${var}' is not set" >&2
    exit 1
  fi
done

# Create the config file with restrictive permissions (0600) before writing
# any content, so passwords are never visible to other processes even
# momentarily during the heredoc write.
install -m 0600 /dev/null /etc/patroni.yml

cat > /etc/patroni.yml << EOF
scope: ${PATRONI_SCOPE}
name: ${PATRONI_NAME}

restapi:
  listen: ${PATRONI_RESTAPI_LISTEN:-0.0.0.0:8008}
  connect_address: ${PATRONI_RESTAPI_CONNECT_ADDRESS}

etcd3:
  hosts: ${PATRONI_ETCD3_HOSTS}

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 10
    maximum_lag_on_failover: 1048576
    postgresql:
      use_pg_rewind: true
      parameters:
        wal_level: replica
        hot_standby: "on"
        max_wal_senders: 10
        max_replication_slots: 10
        wal_log_hints: "on"

  initdb:
    - encoding: UTF8
    - data-checksums

  pg_hba:
    - host replication replicator 172.16.0.0/12 scram-sha-256
    - host all all 172.16.0.0/12 scram-sha-256

postgresql:
  listen: ${PATRONI_POSTGRESQL_LISTEN:-0.0.0.0:5432}
  connect_address: ${PATRONI_POSTGRESQL_CONNECT_ADDRESS}
  data_dir: /var/lib/postgresql/data/pgdata
  bin_dir: /usr/lib/postgresql/16/bin
  authentication:
    replication:
      username: ${PATRONI_REPLICATION_USERNAME:-replicator}
      password: ${PATRONI_REPLICATION_PASSWORD}
    superuser:
      username: ${PATRONI_SUPERUSER_USERNAME:-postgres}
      password: ${PATRONI_SUPERUSER_PASSWORD}
EOF

exec patroni /etc/patroni.yml
