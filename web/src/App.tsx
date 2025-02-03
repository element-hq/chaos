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
import { FederationEdge, ClientServerEdgeLabel, AppEdge } from './Edges';
import type { NodeTypes } from '@xyflow/react';
import { AppNode, ClientNode, HomeserverNode } from './Nodes';
import { ChaosPanel } from './ChaosPanel';
import { ChaosStore, useStore } from './ChaosStore';
import { useShallow } from 'zustand/react/shallow';

const nodeTypes = {
  'homeserver-node': HomeserverNode,
  "client-node": ClientNode,
} satisfies NodeTypes;

const edgeTypes = {
  federation: FederationEdge
} satisfies EdgeTypes;

const selector = (state: ChaosStore) => ({
  nodes: state.nodes,
  edges: state.edges,
  onNodesChange: state.onNodesChange,
  onEdgesChange: state.onEdgesChange,
  onConnect: state.onConnect,
});

// Run  the app
export default function App() {
  const { nodes, edges, onNodesChange, onEdgesChange, onConnect } = useStore(
    useShallow(selector),
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
