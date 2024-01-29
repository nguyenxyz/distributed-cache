## Initial Architecture


![cluster (9)](https://github.com/ph-ngn/nanobox/assets/93941060/6634b5a4-4f7a-4f45-87a5-4513cce8ad63)




## Tech roadmap:
- Raft consensus library: https://github.com/hashicorp/raft?tab=readme-ov-file
- gRPC: https://grpc.io/
- OpenTelemetry: https://opentelemetry.io/
- Zap: https://github.com/uber-go/zap
- Grafana (Loki + Tempo): https://grafana.com/
- InfluxDB: https://www.influxdata.com/
- Docker, K8s, Helm, AWS

## Design:
### nbox-api-server:
-  Summary: Entry point into the box cluster
### cache:
-  Summary: Key-Value Cache
### fsm: 
-  Summary: State machine replication
### kube:
-  Summary: Kubernetes middleware


## Challenges during implementation:
- Sync lru evictions
  + Read requests on replicas will update their lru independently of one another and the master. When capacity is reached, every node will likely have different set of evicted keys which leads to divergence
  + One solution could be disabling eviction in replicas and sync evictions from master with delete requests to replicas. Read requests to replicas can still return evicted entries for some period due to delay, but eventual consistency is still better than a mess :)
