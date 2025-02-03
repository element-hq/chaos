import React, { ReactElement, useEffect, useReducer, useRef, useState } from 'react';
import { BaseEdge, Edge, EdgeLabelRenderer, getBezierPath, getSmoothStepPath, type EdgeProps } from '@xyflow/react';
import { useStore } from './ChaosStore';
import { PayloadFederationRequest } from './WebSockets';

export type FederationEdge = Edge<{ duration: number }, 'federation'>;

export type FederationEdgeData = {
    domain: string
};

// Define nodes and edges
export type AppEdge = FederationEdge | Edge;

export function FederationEdge({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    markerEnd,
    data,
    label,
}: EdgeProps) {
    const d = data as FederationEdgeData;
    const edgePathParams = {
        sourceX,
        sourceY,
        sourcePosition,
        targetX,
        targetY,
        targetPosition,
    };
    const edgePath = getSpecialPath(edgePathParams, sourceX < targetX ? 25 : -25);
    const fedLatencyMs = useStore((state) => state.fedLatencyMs);
    const [startedAnimations, setStartedAnimations] = useState(new Set<string>());

    const animRefs = useRef({} as Record<string, SVGAnimateMotionElement>);
    const duration = fedLatencyMs + "ms";
    const flightBubbles = [];
    const inflightReqs = useStore((state) => state.inflightFedRequests);
    const fedRequests = [];
    const inflightKeys = new Set<string>();
    for (const [id, req] of inflightReqs) {
        const u = URL.parse(req.payload.URL)!;
        if (u.host == d.domain) {
            continue; // another server making a request to us
        }
        const style = {
            fontSize: "smaller",
        };
        fedRequests.push(<div key={id} style={style}>
            {req.payload.Blocked ? "BLOCKED" : u.pathname}
        </div>)
        let colour = d.domain == "hs1" ? "#2050a0" : "#dd3045";
        if (req.payload.Blocked) {
            colour = "#ff0000";
        }
        // bubbles need to be in thier own SVG as SVG's have a global time system.
        // if we try to shove >1 bubble into an SVG then they share the same animation time, so they are
        // always at their end poistion after fedLatencyMs
        flightBubbles.push(
            <svg key={id} x={req.payload.Blocked ? sourceX : undefined} y={req.payload.Blocked ? sourceY : undefined}>
                <circle r="10" fill={colour}>
                    {!req.payload.Blocked && <animateMotion ref={(el) => {
                        animRefs.current[id] = el! as SVGAnimateElement;
                    }} dur={duration} repeatCount="1" fill="freeze" path={edgePath} />}
                </circle>
            </svg>
        );
        inflightKeys.add(id);
    }

    // We need to call beginElement on the animateMotion exactly once per element.
    // We do this by diffing the inflight keys to find newly added elements.
    const diff = inflightKeys.difference(startedAnimations);
    if (diff.size > 0) {
        setStartedAnimations(inflightKeys);
        // Ideally we wouldn't need to do this. In fact, if you hook up inflight requests to a button click then you
        // DON'T need this. But when it's async, via WSes or sleep(1) then there is a race condition where the animation
        // does not start. To fix this, we explicitly ask for a animation frame then call beginElement on the animateMotion.
        setTimeout(() => {
            requestAnimationFrame(() => {
                for (const animKey of diff) {
                    animRefs.current[animKey]?.beginElement();
                }
            });
        }, 0);
    }
    return (
        <>
            <BaseEdge path={edgePath} markerEnd={markerEnd} label={label} />
            {flightBubbles}
            <EdgeLabelRenderer>
                <FederationEdgeLabel
                    transform={`translate(0%, 50%) translate(${sourceX}px,${sourceY + 40}px)`}
                    children={fedRequests}
                />
            </EdgeLabelRenderer>
        </>
    );
}



export function ClientServerEdgeLabel(props: { userId: string }) {
    const clients = useStore((state) => state.clients);
    let client = clients[props.userId] || { action: "-" };
    return (
        <>
            {client.action}
        </>
    );
}

type GetSpecialPathParams = {
    sourceX: number;
    sourceY: number;
    targetX: number;
    targetY: number;
};
const getSpecialPath = (
    { sourceX, sourceY, targetX, targetY }: GetSpecialPathParams,
    offset: number,
) => {
    const centerX = (sourceX + targetX) / 2;
    const centerY = (sourceY + targetY) / 2;

    return `M ${sourceX} ${sourceY} Q ${centerX} ${centerY + offset
        } ${targetX} ${targetY}`;
};

function FederationEdgeLabel({ transform, children }: { transform: string; children: Array<ReactElement> }) {
    return (
        <div
            style={{
                position: 'absolute',
                background: 'transparent',
                padding: 0,
                color: '#ff5050',
                fontSize: 12,
                fontWeight: 700,
                transform,
            }}
            className="nodrag nopan"
        >
            {children}
        </div>
    );
}