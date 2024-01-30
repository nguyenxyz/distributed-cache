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


## Challenges during implementation:
- Sync lru evictions
  + Read requests on replicas will update their lru independently of one another and the master. When capacity is reached, every node will likely have different sets of evicted keys which leads to divergence
  + One solution could be to disable eviction on replicas and sync evictions from the master with delete request to replicas. Read requests to replicas should also be converted to peek to reserve the least recently "write" order so that when the current master goes down and a new master is elected, the order remains consistent across all nodes. This also means that the lru property is only guaranteed when read is forwarded to the master, otherwise only the least-recently write property is reserved (not taking into account the evicted entries)
  + This also comes with some limitations.
      1. Read requests to replicas can return evicted entries due to delay
      2. Master won't know about key access statistics of replicas and so it won't be able update its lru accordingly which is very expensive anyway
  + With that said, we can only provide an approximation of the lru property for performance and consistency reasons. However, arguably if reads for a key happen on both master and replica nodes, the key is a hot key, and since reads should be uniformly distributed on all nodes, this shouldn't be a problem as a hot key is very unlikely to be evicted by the master. Wrong evictions of cold keys shouldn't be a big problem if we can gain significant performance in return.
