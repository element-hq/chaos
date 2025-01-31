import { useCallback } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  addEdge,
  useNodesState,
  useEdgesState,
  type OnConnect,
  EdgeTypes,
  Edge,
} from '@xyflow/react';

import '@xyflow/react/dist/style.css';
import { FederationEdge, ClientServerEdgeLabel } from './Edges';
import type { NodeTypes } from '@xyflow/react';
import { AppNode, ClientNode, HomeserverNode } from './Nodes';
import { ChaosPanel } from './ChaosPanel';

const nodeTypes = {
  'homeserver-node': HomeserverNode,
  "client-node": ClientNode,
} satisfies NodeTypes;


// Define nodes and edges
export type AppEdge = FederationEdge | Edge;

const initialNodes: AppNode[] = [
  {
    id: 'hs1', type: 'homeserver-node', position: { x: -300, y: 100 },
    data: {
      isRestarting: false,
      domain: "hs1",
    }
  },
  {
    id: 'hs2', type: 'homeserver-node', position: { x: 300, y: 100 },
    data: {
      isRestarting: false,
      domain: "hs2",
    }
  },
  { id: "client1", type: "client-node", position: { x: -300, y: -100 }, data: { domain: "hs1" } },
  { id: "client2", type: "client-node", position: { x: 300, y: -100 }, data: { domain: "hs2" } },
];
const initialEdges: AppEdge[] = [
  { id: 'hs1hs2', source: 'hs1', target: 'hs2', sourceHandle: "federationR", targetHandle: "federationL", label: "hs1", type: "federation", data: { domain: "hs1" } },
  { id: 'hs2hs1', source: 'hs2', target: 'hs1', sourceHandle: "federationL", targetHandle: "federationR", label: "hs2", type: "federation", data: { domain: "hs2" } },
  { id: "hs1-client1", source: "client1", target: "hs1", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs1" /> },
  { id: "hs2-client1", source: "client2", target: "hs2", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs2" /> },
];

const edgeTypes = {
  federation: FederationEdge
} satisfies EdgeTypes;

// Run  the app
export default function App() {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const onConnect: OnConnect = useCallback(
    (connection) => setEdges((edges) => addEdge(connection, edges)),
    [setEdges]
  );

  return (
    <ReactFlow
      nodes={nodes}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      edges={edges}
      edgeTypes={edgeTypes}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      fitView
    >
      <Background />
      <Controls />
      <ChaosPanel />

    </ReactFlow>
  );
}
