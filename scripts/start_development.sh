#####
# SQL
#####

# deploy a local distributed sql database that is cloud native by default. NOTE: this is for local development only
# for cluster deployments please the deployment manifest.
SQL_DB_NETWORK=sql
MASTER_NODE=sql-db-1
NODE_2=sql-db-2
NODE_3=sql-db-3

DB_USER=firegraph
DB_NAME=firegraph

# build the image is needed
docker build -t db ./deployments/sql/

# create a docker network for db images to communicate on
# then deploy three nodes, with one being the master in insecure mode

# create docker cockraoch network bridge
docker network create -d bridge ${SQL_DB_NETWORK}

# create first node
docker run -d --name=${MASTER_NODE} --hostname=${MASTER_NODE} --network=${SQL_DB_NETWORK} -p 26257:26257 -p 9000:8080 -v "${PWD}/deployments/sql/data/${MASTER_NODE}:/cockroach/cockroach-data" db start --insecure
docker run -d --name=${NODE_2} --hostname=${NODE_2} --network=${SQL_DB_NETWORK} -v "${PWD}/deployments/sql/data/${NODE_2}:/cockroach/cockroach-data" db start --insecure --join=${MASTER_NODE}
docker run -d --name=${NODE_3} --hostname=${NODE_3} --network=${SQL_DB_NETWORK} -v "${PWD}/deployments/sql/data/${NODE_3}:/cockroach/cockroach-data" db start --insecure --join=${MASTER_NODE}

# check status
docker ps
docker network ls

# the database server is running so add a user, and create the database
docker exec -it ${MASTER_NODE} ./cockroach sql --insecure \
  --execute "CREATE USER IF NOT EXISTS ${DB_USER};" \
  --execute "CREATE DATABASE ${DB_NAME};" \
  --execute "GRANT ALL ON DATABASE ${DB_NAME} TO ${DB_USER};"

#######
#BROKER
#######
BROKER_NETWORK=broker
BROKER_SERVER_NAME=broker-leader
BROKER_WORK_1=broker-worker-1
BROKER_WORK_2=broker-worker-2
BROKER_CLUSTER_NAME=NATS

# build alpine image for broker
docker build -t broker ./deployments/broker/

# build broker network
docker network create -d bridge ${BROKER_NETWORK}

# run nats server
docker run -d --name ${BROKER_SERVER_NAME} --network ${BROKER_NETWORK} -p 4222:4222 -p 8222:8222 nats --http_port 8222 --cluster_name ${BROKER_CLUSTER_NAME} --cluster nats://0.0.0.0:6222

# pause giving the server time to startup
sleep 10

# run nats worker nodes, and join them with the leader
docker run -d --name ${BROKER_WORK_1} --network ${BROKER_NETWORK} nats --cluster_name ${BROKER_CLUSTER_NAME} --cluster nats://0.0.0.0:6222 --routes=nats://ruser:T0pS3cr3t@nats:6222
docker run -d --name ${BROKER_WORK_2} --network ${BROKER_NETWORK} nats --cluster_name ${BROKER_CLUSTER_NAME} --cluster nats://0.0.0.0:6222 --routes=nats://ruser:T0pS3cr3t@nats:6222


