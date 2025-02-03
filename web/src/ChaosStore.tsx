import { create } from 'zustand';
import { type AppNode } from "./Nodes";
import { ChaosWebsocket, PayloadConfig, PayloadConvergence, PayloadFederationRequest, PayloadNetsplit, PayloadRestart, PayloadTickGeneration, PayloadWorkerAction } from './WebSockets';
import { addEdge, applyEdgeChanges, applyNodeChanges, Connection, Edge, EdgeChange, NodeChange, Position } from '@xyflow/react';
import { AppEdge, ClientServerEdgeLabel } from './Edges';

export type ChaosStore = {
    convergenceState: string
    isNetsplit: boolean
    started: boolean
    fedLatencyMs: number
    inflightFedRequests: Map<string, {
        payload: PayloadFederationRequest,
        start: number,
    }>

    // user_id => action
    clients: Record<string, {
        action: string,
    }>
    tickNumber: number

    connectedToRemoteServer: boolean
    serversRestarting: Set<string>

    nodes: AppNode[]
    edges: AppEdge[]

    onConnected: (payload: PayloadConfig) => void
    onWorkerAction: (payload: PayloadWorkerAction) => void
    onTickGeneration: (payload: PayloadTickGeneration) => void
    onConvergenceUpdate: (payload: PayloadConvergence) => void
    onNetsplit: (payload: PayloadNetsplit) => void
    onFederationRequest: (payload: PayloadFederationRequest) => void
    onServerRestart: (payload: PayloadRestart) => void


    connect: (wsAddr: string) => void
    start: () => void
    restart: (hsName: string) => void
    netsplitToggle: () => void
    testConvergence: () => void

    onNodesChange: (changes: NodeChange<AppNode>[]) => void
    onEdgesChange: (changes: EdgeChange<AppEdge>[]) => void
    onConnect: (connection: Edge | Connection) => void
    setNodes: (nodes: AppNode[]) => void
    setEdges: (edges: AppEdge[]) => void

}


function sleep(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

let ws = new ChaosWebsocket();
let addedListeners = false;

export const useStore = create<ChaosStore>()((set, get) => ({
    convergenceState: "-",
    started: false,
    isNetsplit: false,
    connectedToRemoteServer: false,
    tickNumber: 0,
    fedLatencyMs: 1000,
    clients: {},
    inflightFedRequests: new Map(),
    serversRestarting: new Set<string>(),
    nodes: [
        {
            id: 'hs1', type: 'homeserver-node', position: { x: -300, y: 100 },
            data: { domain: "hs1" }
        },
        {
            id: 'hs2', type: 'homeserver-node', position: { x: 300, y: 100 },
            data: { domain: "hs2" }
        },
        //{ id: "client1", type: "client-node", position: { x: -300, y: -100 }, data: { domain: "hs1" } },
        //{ id: "client2", type: "client-node", position: { x: 300, y: -100 }, data: { domain: "hs2" } },
    ],
    edges: [
        { id: 'hs1hs2', source: 'hs1', target: 'hs2', sourceHandle: "federationR", targetHandle: "federationL", label: "hs1", type: "federation", data: { domain: "hs1" } },
        { id: 'hs2hs1', source: 'hs2', target: 'hs1', sourceHandle: "federationL", targetHandle: "federationR", label: "hs2", type: "federation", data: { domain: "hs2" } },
        //{ id: "hs1-client1", source: "client1", target: "hs1", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs1" /> },
        //{ id: "hs2-client1", source: "client2", target: "hs2", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs2" /> },
    ],

    // Server-received actions (mapped from WS payloads)
    // -------------------------------------------------

    onConnected: (payload: PayloadConfig) => {
        console.log("onConnected ", payload);
        if (payload.Config.Homeservers.length !== 2) {
            console.error("Incompatible Chaos configuration, only 2 HSes are supported currently.");
            console.error("Configuration received:", payload.Config);
        }
        if (payload.Config.Test.FederationDelayMs) {
            set({
                fedLatencyMs: payload.Config.Test.FederationDelayMs,
            });
        }
        const clients: Record<string, {
            action: string,
        }> = {};
        const nodes = get().nodes;
        const edges = get().edges;
        const usersByDomain = new Map<string, string[]>();
        for (const userId of payload.WorkerUserIDs) {
            const domain = userId.split(":")[1];
            const entries = usersByDomain.get(domain) || [];
            entries.push(userId);
            usersByDomain.set(domain, entries);
        }
        for (const [domain, userIds] of usersByDomain) {
            for (let i = 0; i < userIds.length; i++) {
                const userId = userIds[i];
                const domain = userId.split(":")[1];
                clients[userId] = {
                    action: "-",
                };
                nodes.push({
                    id: userId,
                    type: "client-node",
                    position: { x: domain === "hs1" ? -600 : 600, y: -150 + (i * 150) },
                    data: { userId: userId, position: domain === "hs1" ? Position.Right : Position.Left },
                } as AppNode);
                edges.push({
                    id: userId + "-edge",
                    source: userId,
                    target: domain,
                    animated: true,
                    type: "default",
                    label: <ClientServerEdgeLabel userId={userId} />
                });
            }
        }
        console.log("setting", clients);
        set({
            clients: clients,
            // need to copy else reactflow blows up as I guess it does a shallow check
            // and doesn't update its internal state otherwise.
            nodes: Array.from(nodes),
            edges: Array.from(edges),
        });
    },
    onWorkerAction: (payload: PayloadWorkerAction) => {
        const client = get().clients[payload.UserID];
        client.action = `${payload.Action} ${payload.Body ? payload.Body : ""}`;
        set({
            clients: {
                ...get().clients,
                [payload.UserID]: client,
            }
        })
    },
    onTickGeneration: (payload: PayloadTickGeneration) => {
        set({
            tickNumber: payload.Number
        });
    },
    onConvergenceUpdate: (payload: PayloadConvergence) => {
        if (payload.Error) {
            set({
                convergenceState: "ERROR: " + payload.Error,
            });
        } else {
            set({
                convergenceState: payload.State,
            });
        }
    },
    onNetsplit: (payload: PayloadNetsplit) => {
        set({
            isNetsplit: payload.Started,
        });
    },
    onFederationRequest: (payload: PayloadFederationRequest) => {
        useStore.setState((prev) => {
            console.log(payload.ID, payload.URL, "existing: ", prev.inflightFedRequests.size);
            const copy = new Map(prev.inflightFedRequests);
            copy.set(payload.ID, {
                payload: payload,
                start: Date.now(),
            });
            setTimeout(() => {
                useStore.setState((prev) => {
                    const copy = new Map(prev.inflightFedRequests);
                    copy.delete(payload.ID);
                    return {
                        inflightFedRequests: copy,
                    };
                })
            }, prev.fedLatencyMs);
            return {
                inflightFedRequests: copy,
            };
        });
    },
    onServerRestart: (payload: PayloadRestart) => {
        useStore.setState((prev) => {
            const copy = new Set<string>(prev.serversRestarting);
            if (payload.Finished) {
                copy.delete(payload.Domain);
            } else {
                copy.add(payload.Domain);
            }
            console.log("Servers restarting: ", Array.from(copy));
            return {
                serversRestarting: copy,
            };
        });
    },



    // End-user issued actions
    // -----------------------

    connect: async (wsAddr: string): Promise<void> => {
        console.log("connect ", wsAddr);
        if (!addedListeners) {
            ws.addEventListener("PayloadConfig", (ev: unknown) => {
                set({
                    connectedToRemoteServer: true,
                });
                get().onConnected((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadWorkerAction", (ev: unknown) => {
                get().onWorkerAction((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadTickGeneration", (ev: unknown) => {
                get().onTickGeneration((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadConvergence", (ev: unknown) => {
                get().onConvergenceUpdate((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadNetsplit", (ev: unknown) => {
                get().onNetsplit((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadFederationRequest", (ev: unknown) => {
                get().onFederationRequest((ev as CustomEvent).detail);
            });
            ws.addEventListener("PayloadRestart", (ev: unknown) => {
                get().onServerRestart((ev as CustomEvent).detail);
            });
            addedListeners = true;
        }
        await ws.connect(wsAddr);
    },
    start: async (): Promise<void> => {
        console.log("start");
        ws.start();
    },
    restart: (hsName: string): void => {
        console.log("restart ", hsName);
        ws.setRestart(hsName);
    },
    netsplitToggle: (): void => {
        console.log("netsplitToggle");
        ws.setNetsplit(!get().isNetsplit);
    },
    testConvergence: (): void => {
        ws.testConvergence();
    },

    // Reactflow functions
    // -------------------

    onNodesChange: (changes) => {
        set({
            nodes: applyNodeChanges(changes, get().nodes),
        });
    },
    onEdgesChange: (changes) => {
        set({
            edges: applyEdgeChanges(changes, get().edges),
        });
    },
    onConnect: (connection) => {
        set({
            edges: addEdge(connection, get().edges),
        });
    },
    setNodes: (nodes) => {
        set({ nodes });
    },
    setEdges: (edges) => {
        set({ edges });
    },
}));
