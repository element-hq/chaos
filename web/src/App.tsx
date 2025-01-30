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
import { AnimatedSVGEdge, ClientServerEdgeLabel } from './Edges';
import type { NodeTypes } from '@xyflow/react';
import { AppNode, ClientNode, HomeserverNode } from './Nodes';
import { ChaosPanel } from './ChaosPanel';

const nodeTypes = {
  'homeserver-node': HomeserverNode,
  "client-node": ClientNode,
} satisfies NodeTypes;


// Define nodes and edges
export type AppEdge = AnimatedSVGEdge | Edge;

const initialNodes: AppNode[] = [
  {
    id: 'hs1', type: 'homeserver-node', position: { x: -200, y: 100 },
    data: {
      label: 'hs1',
      isRestarting: false,
    }
  },
  {
    id: 'hs2', type: 'homeserver-node', position: { x: 200, y: 100 },
    data: {
      label: 'hs2',
      isRestarting: false,
    }
  },
  { id: "client1", type: "client-node", position: { x: -200, y: -100 }, data: { domain: "hs1" } },
  { id: "client2", type: "client-node", position: { x: 200, y: -100 }, data: { domain: "hs2" } },
];
const initialEdges: AppEdge[] = [
  { id: 'hs1hs2', source: 'hs1', target: 'hs2', targetHandle: "federation", animated: false, label: "foo", type: "animatedSvg", data: { duration: "1s" } },
  { id: "hs1-client1", source: "client1", target: "hs1", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs1" /> },
  { id: "hs2-client1", source: "client2", target: "hs2", animated: true, type: "default", label: <ClientServerEdgeLabel domain="hs2" /> },
];

const edgeTypes = {
  animatedSvg: AnimatedSVGEdge
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
