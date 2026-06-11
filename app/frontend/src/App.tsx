import {useEffect, useState} from 'react';
import './App.css';
import {GetAppStatus} from "../wailsjs/go/main/App";
import {main} from "../wailsjs/go/models";

const pages = [
    {id: "dashboard", label: "儀表板"},
    {id: "scan", label: "掃描"},
    {id: "results", label: "結果"},
    {id: "schedule", label: "排程"},
    {id: "quarantine", label: "隔離區"},
    {id: "settings", label: "設定"},
    {id: "logs", label: "紀錄"},
    {id: "about", label: "關於"},
];

const statusCards = [
    {label: "Runtime", value: "App-managed 共用", detail: "系統層共用 Runtime、database 與 daemon"},
    {label: "掃描引擎", value: "clamd + INSTREAM", detail: "不靜默降級為 clamscan"},
    {label: "使用者資料", value: "各使用者分離", detail: "設定、jobs、結果與隔離區各自保存"},
];

function App() {
    const [activePage, setActivePage] = useState("dashboard");
    const [status, setStatus] = useState<main.AppStatus | null>(null);

    useEffect(() => {
        GetAppStatus().then(setStatus);
    }, []);

    function renderPage() {
        if (activePage === "dashboard") {
            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">系統狀態</p>
                            <h2>儀表板</h2>
                        </div>
                        <button className="primaryButton" type="button">安裝 Runtime</button>
                    </div>
                    <div className="statusGrid">
                        {statusCards.map((card) => (
                            <article className="statusCard" key={card.label}>
                                <span>{card.label}</span>
                                <strong>{card.value}</strong>
                                <p>{card.detail}</p>
                            </article>
                        ))}
                    </div>
                    <div className="runtimeBox">
                        <div>
                            <span>Runtime 模式</span>
                            <strong>{formatRuntimeMode(status?.runtime.mode)}</strong>
                        </div>
                        <div>
                            <span>Socket</span>
                            <code>{status?.runtime.clamdSocket ?? "載入中"}</code>
                        </div>
                        <div>
                            <span>Database</span>
                            <code>{status?.runtime.databasePath ?? "載入中"}</code>
                        </div>
                    </div>
                </section>
            );
        }

        if (activePage === "settings") {
            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">控制項</p>
                            <h2>設定</h2>
                        </div>
                    </div>
                    <div className="settingsGrid">
                        <label><span>排程掃描</span><input type="checkbox"/></label>
                        <label><span>Database 更新</span><input type="checkbox" defaultChecked/></label>
                        <label><span>電池供電時執行</span><input type="checkbox"/></label>
                        <label><span>省電模式下執行</span><input type="checkbox"/></label>
                        <label><span>登入時啟動</span><input type="checkbox"/></label>
                        <label><span>保留狀態列 icon</span><input type="checkbox" defaultChecked/></label>
                    </div>
                </section>
            );
        }

        const page = pages.find((item) => item.id === activePage);

        return (
            <section className="panel emptyPanel">
                <p className="eyebrow">{page?.label ?? "頁面"}</p>
                <h2>{page?.label ?? "頁面"}</h2>
                <p>此工作區已準備好接續下一個實作任務。</p>
            </section>
        );
    }

    return (
        <div className="appShell">
            <aside className="sidebar">
                <div className="brand">
                    <div className="mark">C</div>
                    <div>
                        <strong>ClamAV Desktop</strong>
                        <span>macOS 掃描器</span>
                    </div>
                </div>
                <nav>
                    {pages.map((page) => (
                        <button
                            className={page.id === activePage ? "active" : ""}
                            key={page.id}
                            onClick={() => setActivePage(page.id)}
                            type="button"
                        >
                            {page.label}
                        </button>
                    ))}
                </nav>
            </aside>
            <main className="content">
                <header className="topbar">
                    <div>
                        <h1>ClamAV Desktop</h1>
                        <p>正式版路線：共用 Runtime、共用 daemon、使用者資料分離。</p>
                    </div>
                    <span className="statusPill">{formatRuntimeMode(status?.runtime.mode)}</span>
                </header>
                {renderPage()}
            </main>
        </div>
    );
}

function formatRuntimeMode(mode?: string) {
    if (!mode) {
        return "檢查中";
    }

    if (mode === "missing") {
        return "尚未安裝";
    }

    if (mode === "system-shared") {
        return "系統共用";
    }

    return mode;
}

export default App;
