#!/bin/bash -eu
export SYNAPSE_REPORT_STATS=no

if [ -f /data/homeserver.yaml ]; then
    echo "homeserver.yaml already detected, not regenerating config"
    /start.py run
    exit 0
fi

openssl req -x509 -newkey rsa:4096 \
          -keyout "/data/$SYNAPSE_SERVER_NAME.tls.key" \
          -out "/data/$SYNAPSE_SERVER_NAME.tls.crt" \
          -days 365 -nodes -subj "/O=matrix"


echo " ====== Generating config  ====== "
/start.py generate

# Allow open registration
/yq -i '.enable_registration = true' /data/homeserver.yaml
/yq -i '.enable_registration_without_verification = true' /data/homeserver.yaml

# Disable TLS checks over federation
/yq -i '.federation_verify_certificates = false' /data/homeserver.yaml
/yq -i '.trusted_key_servers = []' /data/homeserver.yaml

# Provide TLS certs for listening on :443
/yq -i ".tls_certificate_path = \"/data/$SYNAPSE_SERVER_NAME.tls.crt\"" /data/homeserver.yaml
/yq -i ".tls_private_key_path = \"/data/$SYNAPSE_SERVER_NAME.tls.key\"" /data/homeserver.yaml

# Listen on :443 and serve up a .well-known response pointing to :443
/yq -i '.listeners = [{"port":443,"tls":true,"type":"http","resources":[{"names":["client","federation"]}]}]' /data/homeserver.yaml
/yq -i ".serve_server_wellknown = true" /data/homeserver.yaml

# set rate limiting stuff
/yq -i '.rc_message = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_registration = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_login.address = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_login.account = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_login.failed_attempts = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_login.address = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_admin_redaction = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_joins.local = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_joins.remote = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_invites.per_room = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_invites.per_user = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_invites.per_issuer = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_3pid_validation = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_joins_per_room = {"per_second":1000,"burst_count":1000}' /data/homeserver.yaml
/yq -i '.rc_federation = {"sleep_delay":1}' /data/homeserver.yaml

echo " ====== Starting server with:  ====== "
cat /data/homeserver.yaml
echo  " ====== STARTING  ====== " 
/start.py run
