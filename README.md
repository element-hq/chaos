<div align="center">
<img src="web/public/chaos-alpha.png" alt="Alt Text" width="100" height="100">
 <h1>Chaos</h1>
</div>

[![Matrix](https://img.shields.io/matrix/chaos-testing:matrix.org)](https://matrix.to/#/#chaos-testing:matrix.org)
[![GitHub License](https://img.shields.io/github/license/element-hq/chaos)](/LICENSE)


Chaos is a partition/fault tolerance testing tool for homeservers. It can cause netsplits and restart servers, and periodically checks for convergence by ensuring all homeservers see the same member list as every other server.

### Quick Start
```
# Node 20+ required
(cd web && yarn install && yarn build)
go build ./cmd/chaos
docker compose up -d
./chaos -config config.demo.yaml --web
```

To access the databases of the demo servers:
```
$ ./demo/database_repl.sh hs1 # or hs2
```

To run in CLI only mode drop `--web`.

### Running

To setup Chaos with your homeservers, you need:
 - to run the homeservers in a single docker network,
 - to set `HTTP_PROXY` and `HTTPS_PROXY` environment variables to the constant `http://mitmproxy`,
 - run a mitmproxy in the same docker network with the container name `mitmproxy` with the
   args: `mitmdump --set  ssl_insecure=true -s /addons/__init__.py` and volume `./mitmproxy_addons:/addons`.

Once you've done this, build and run chaos:
- Build the binary: `go build ./cmd/chaos`.
- Edit the config file: `config.yml`.
- Run it: `./chaos -c config.yml`

Note: The homeservers do NOT need to be Complement-compatible.

### Architecture / Dev Notes

```
       +--------------------------------+
       |             Chaos              |
       +-|-------------^--------------|-+
Host     |             |              |
=========|=============|==============|======
Docker   | CSAPI       |callback      | CSAPI
         V             V addon        V
       +-----+   +-----------+   +-----+
       | hs1 <---> mitmproxy <---> hs2 |
       +-----+   +-----------+   +-----+
```

This uses the same technique that [complement-crypto](https://github.com/matrix-org/complement-crypto/)
uses to intercept CSAPI traffic, but it intercepts _federation traffic_. The same functionality is
supported to manipulate and block traffic.

We use Go to give us the true parallelism vs Python or Node which are single core async runtimes.

#### Design Requirements

- We want concurrent requests to generate load.
- We want the internal state machine to be simple enough that tests know what the expected end state is.
- We want to generate state and messages in rooms.
- We want to exercise state resolution.
- We want to be able to netsplit servers.
- We want to be as deterministic as possible without sacrificing concurrency.
- We want to visualise traffic and netsplits.

#### Design

- WORKERS: Each user is a goroutine. Each goroutine gets HTTP request instructions, performs it and returns OK or an error.
  * technically we want N worker goroutines, and bucket users by worker, but for the numbers we're talking about (<100 users) 1 worker per user is fine.
  * we want the "Master goroutine" which sends instructions to maintain the confirmed/pending transitions, hence why each goroutine is dumb and doesn't have any internal state themselves.
  * Setting workers=1 means we have at most 1 in-flight req, and provided we order instructions deterministically, we will get deterministic-ish runs, but sacrifice concurrency to do so.
- MASTER: A Master goroutine which knows the entire test state. Master prepares the test by creating users/rooms up-front. We don't want to faff with that in our state machine. Master creates instructions:
  * State machine for a user and room:
    ```
                        .---. 
                        V   |
    START---->JOIN---->SEND_MSG---->LEAVE
                ^                     ^
                `---------------------`
    ```
  * Each transition has a probability, and the dice roll is obtained from a PRNG with a fixed seed.
  * We need to execute instructions in sequence (e.g sorted by user ID) for determinism.
- TICKS: To get limited determinism with concurrency, we split instructions into groups called "ticks". Each tick, the Master makes N instructions and concurrently tells workers to execute them. The Master then waits for the responses before proceeding to the next tick. This will create spikey traffic as we wait for the long tail of requests to respond.
  * At the end of each tick, we can perform a "snapshot" or test for "convergence".
- SNAPSHOT: Collects metrics about the running servers.
- CONVERGENCE: Queries room state on all servers to ensure state has converged (HS1==HS2==Master) - that is all servers and Chaos agree on the member list for the room. Requires some synchronisation between servers first as some servers may have lost traffic due to "netsplits".
- NETSPLITS: Can occur at any time. In CLI mode this is on a timer, in Web mode it's user controlled. Netsplits should never cause state transitions to fail (e.g joining a room), as we are highly available and masters are always joined to the room so the server always has at least 1 user joined.
