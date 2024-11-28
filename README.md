## Chaos

Chaos is a partition/fault tolerance testing tool for homeservers. It can deterministically cause netsplits and restart servers, and periodically checks for convergence by ensuring all homeservers see the same member list as every other server.

### Quick Start
```
go build ./cmd/chaos
docker compose up -d
./chaos -config config.demo.yaml
```

### Running

To setup Chaos with your homeservers, you need:
 - to run the homeservers in a single docker network,
 - to set `HTTP_PROXY` and `HTTPS_PROXY` environment variables to the constant `http://mitmproxy`,
 - run a vanilla mitmproxy in the same docker network with the container name `mitmproxy`.

Once you've done this, build and run chaos:
- Build the binary: `go build ./cmd/chaos`.
- Edit the config file: `config.yml`.
- Run it: `./chaos -c config.yml`

Note: The homeservers do NOT need to be Complement-compatible. To run the demo: `docker-compose up` which:
 - spins up two homeservers with env vars set
 - spins up a mitmproxy
then `./chaos -config config.demo.yaml`.

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

We use Go tests to give us the required amount of concurrency (multi-core, async runtimes) vs Python
or Node which are single core async runtimes.

#### Requirements

- We want concurrent requests to generate load.
- We want the internal state machine to be simple enough that tests know what the expected end state is.
- We want to generate state and messages in rooms.
- We want to exercise state resolution.
- We want to be able to netsplit servers.
- We want to be deterministic without sacrificing concurrency.
- Stretch: We may want to visualise traffic and netsplits.

#### Design

- WORKERS: Each user is a goroutine. Each goroutine gets HTTP request instructions, performs it and returns OK or an error.
  * technically we want N worker goroutines, and bucket users by worker, but for the numbers we're talking about (<100 users) 1 worker per user is fine.
  * we want the "Master goroutine" which sends instructions to maintain the confirmed/pending transitions, hence why each goroutine is dumb and doesn't have any internal state themselves.
  * Setting workers=1 means we have at most 1 in-flight req, and provided we order instructions deterministically, we will get deterministic runs, but sacrifice concurrency to do so.
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
  * We need to execute users in sequence (e.g sorted by user ID) for determinism.
- TICKS: To get limited determinism with concurrency, we split instructions into groups called "ticks". Each tick, the Master makes N instructions and concurrently tells workers to execute them. The Master then waits for the responses before proceeding to the next tick. This will create spikey traffic as we wait for the long tail of requests to respond.
  * At the end of each tick, we perform a "snapshot".
- SNAPSHOT: Collects pod metrics and whines about any errors. Can also query room state on all servers to ensure state has converged (HS1==HS2==Master)
- NETSPLITS: Can occur at the start of a tick (same PRNG for users is used). Lasts the duration of the tick (TODO: make configurable?).
  * In theory this could be done at any point but we want determinism so pin it to the start of a tick.
  * Netsplits should never cause state transitions to fail, as we are highly available. The only caveat would be if all 1 server's users have left a room, so maybe need a sentinel user (Master user IDs) to be joined to all rooms to just lurk.
