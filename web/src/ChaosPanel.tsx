import { Panel } from "@xyflow/react";
import { useStore } from "./ChaosStore";
import { useState } from "react";
import { useShallow } from "zustand/react/shallow";


function InfoBox(props: { buttonName: string, label: string, onClick: () => void }) {
    return <div className="infobox">
        <input className="infobox-btn" type="button" value={props.buttonName} onClick={props.onClick} />
        <span className="infobox-label">{props.label}</span>
    </div>
}

export function ChaosPanel() {
    const store = useStore( // stops every update to the store re-rendering the panel
        useShallow((state) => ({
            connect: state.connect,
            start: state.start,
            netsplitToggle: state.netsplitToggle,
            testConvergence: state.testConvergence,
            connectedToRemoteServer: state.connectedToRemoteServer,
            tickNumber: state.tickNumber,
            isNetsplit: state.isNetsplit,
            convergenceState: state.convergenceState,
        })),
    );
    let [wsURL, setWsURL] = useState("http://localhost:7405")

    return <Panel position="top-left">
        <div id="chaos-panel">
            <img src="chaos-alpha.png" id="chaos-img" width="100px" />
            <input type="input" placeholder="Chaos WS URL" value={wsURL} onChange={(e) => setWsURL(e.target.value)} />
            <input type="button" value="Connect" onClick={() => store.connect(wsURL)} />
            {store.connectedToRemoteServer ? <>
                <InfoBox buttonName="Start" label={"Tick: " + store.tickNumber} onClick={() => store.start()} />
                <InfoBox buttonName="Netsplit" label={store.isNetsplit ? "Disconnected" : "Connected"} onClick={() => store.netsplitToggle()} />
                <InfoBox buttonName="Test" label={store.convergenceState} onClick={() => store.testConvergence()} />
            </> : <></>}
        </div>
    </Panel>
};