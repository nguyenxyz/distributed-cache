## Cluster Infra Design


![cluster](https://github.com/ph-ngn/nanobox/assets/93941060/e05533af-a200-43fd-b775-79923fcabe3a)

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
  + One solution could be to disable eviction on replicas and sync evictions from the master with delete request to replicas. This also means that the lru property is only guaranteed when read is forwarded to the master. With that said, we can only provide an approximation of the lru property for performance and consistency reasons. However, arguably if reads for a key happen on both master and replica nodes, the key is a hot key, and since reads should be uniformly distributed on all nodes, this shouldn't be a problem as a hot key is very unlikely to be evicted by the master. Wrong evictions of cold keys shouldn't be a big problem if we can gain significant performance in return.
  + This also comes with some limitations.
    1. Read requests to replicas can return evicted entries due to delay. In any case, reads are eventually consistentent if enabled on replicas
    2. Master won't know about key access statistics of replicas and so it won't be able update its lru accordingly which is very costly anyway


## Demo:
<img width="1091" alt="Screenshot 2024-05-20 at 5 44 54 PM" src="https://github.com/ph-ngn/nanobox/assets/93941060/85f627da-da26-49fc-822c-42e1221a2be3">
<img width="972" alt="Screenshot 2024-05-20 at 5 45 40 PM" src="https://github.com/ph-ngn/nanobox/assets/93941060/277cb951-b641-420b-81d7-f0485265ab3a">

  
