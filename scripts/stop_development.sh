# CLEANUP SQL
docker stop sql-db-1
docker stop sql-db-2
docker stop sql-db-3

docker rm sql-db-1 
docker rm sql-db-2
docker rm sql-db-3

docker network rm sql

sudo rm -rf $PWD/deployments/sql/data/

# CLEANUP BROKER
docker stop broker-worker-1
docker stop broker-worker-2
docker stop broker-leader

docker rm broker-worker-1
docker rm broker-worker-2
docker rm broker-leader

docker network rm broker
