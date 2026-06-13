import {useEffect, useState} from 'react';
import './App.css';
import appIcon from './assets/appicon.png';
import {
    CancelScanJob,
    ExportDiagnostics,
    GetAppStatus,
    GetAboutInfo,
    GetCommonScanPaths,
    GetLoginItemStatus,
    GetRuntimeSetupStatus,
    GetSettings,
    GetSystemPermissionStatus,
    ListAppLogEntries,
    ListQuarantineRecords,
    ListScanJobs,
    LoadScanResults,
    MoveQuarantineRecordToTrash,
    MoveScanResultToTrash,
    OpenFullDiskAccessSettings,
    OpenNotificationSettings,
    OpenQuarantineLocation,
    OpenScanResultLocation,
    PermanentlyDeleteQuarantineRecord,
    PermanentlyDeleteScanResult,
    QuarantineScanResult,
    ReadClamdLog,
    ReadFreshclamLog,
    RemoveUserData,
    RestoreQuarantineRecord,
    SaveSettings,
    SelectScanFiles,
    SelectScanFolder,
    StartScan,
    UpdateDatabase
} from "../wailsjs/go/main/App";
import {main} from "../wailsjs/go/models";
import {BrowserOpenURL, CheckNotificationAuthorization, EventsOn} from "../wailsjs/runtime/runtime";

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

const weekdays = [
    {value: 0, label: "週日"},
    {value: 1, label: "週一"},
    {value: 2, label: "週二"},
    {value: 3, label: "週三"},
    {value: 4, label: "週四"},
    {value: 5, label: "週五"},
    {value: 6, label: "週六"},
];

type ScanProgressEventPayload = {
    jobId: string;
    status: string;
    currentPath: string;
    scannedFiles: number;
    detections: number;
    errors: number;
};

type FreshclamEventPayload = {
    stream: string;
    message: string;
    at: string;
};

type ToastTone = "info" | "success" | "error" | "pending";

type ToastItem = {
    id: number;
    tone: ToastTone;
    title: string;
    detail?: string;
};

type DialogState = {
    title: string;
    body: string[];
    confirmLabel?: string;
    danger?: boolean;
    onConfirm?: () => void;
};

type PermissionDisplayStatus = {
    status: string;
    message?: string;
};

const RESULTS_PAGE_SIZE = 25;

function App() {
    const [activePage, setActivePage] = useState("dashboard");
    const [status, setStatus] = useState<main.AppStatus | null>(null);
    const [aboutInfo, setAboutInfo] = useState<main.AboutInfo | null>(null);
    const [runtimeSetup, setRuntimeSetup] = useState<main.RuntimeSetupStatus | null>(null);
    const [forceShowSetup, setForceShowSetup] = useState(false);
    const [settings, setSettings] = useState<main.Settings | null>(null);
    const [systemPermissions, setSystemPermissions] = useState<main.SystemPermissionStatus | null>(null);
    const [notificationPermission, setNotificationPermission] = useState<PermissionDisplayStatus>({status: "unknown"});
    const [loginItemStatus, setLoginItemStatus] = useState<main.LoginItemStatus | null>(null);
    const [schedulePathsText, setSchedulePathsText] = useState("");
    const [scanPathsText, setScanPathsText] = useState("");
    const [scanPresets, setScanPresets] = useState<{label: string; path: string}[]>([]);
    const [scanRecursive, setScanRecursive] = useState(true);
    const [scanJob, setScanJob] = useState<main.ScanJob | null>(null);
    const [scanProgress, setScanProgress] = useState<ScanProgressEventPayload | null>(null);
    const [scanResults, setScanResults] = useState<main.ScanResult[]>([]);
    const [scanMessage, setScanMessage] = useState("");
    const [freshclamEvents, setFreshclamEvents] = useState<FreshclamEventPayload[]>([]);
    const [quarantineRecords, setQuarantineRecords] = useState<main.QuarantineRecord[]>([]);
    const [appLogEntries, setAppLogEntries] = useState<main.LogEntry[]>([]);
    const [scanJobs, setScanJobs] = useState<main.ScanJob[]>([]);
    const [freshclamLogLines, setFreshclamLogLines] = useState<string[]>([]);
    const [clamdLogLines, setClamdLogLines] = useState<string[]>([]);
    const [resultsJobs, setResultsJobs] = useState<main.ScanJob[]>([]);
    const [resultsJobId, setResultsJobId] = useState("");
    const [resultsItems, setResultsItems] = useState<main.ScanResult[]>([]);
    const [resultsFilter, setResultsFilter] = useState("all");
    const [toasts, setToasts] = useState<ToastItem[]>([]);
    const [dialog, setDialog] = useState<DialogState | null>(null);
    const [resultsSearch, setResultsSearch] = useState("");
    const [resultsPage, setResultsPage] = useState(1);

    useEffect(() => {
        GetAppStatus().then(setStatus);
        GetSettings().then((loaded) => {
            setSettings(loaded);
            setSchedulePathsText(loaded.scanSchedule.paths.join("\n"));
        });
        loadLoginItemStatus();
        loadRuntimeSetup();
        GetCommonScanPaths().then(setScanPresets).catch(() => setScanPresets([]));
    }, []);

    useEffect(() => {
        if (settings) {
            setSchedulePathsText(settings.scanSchedule.paths.join("\n"));
        }
    }, [settings?.scanSchedule.paths]);

    useEffect(() => {
        const off = EventsOn("scan:progress", (event: ScanProgressEventPayload) => {
            setScanProgress(event);
        });
        return off;
    }, []);

    useEffect(() => {
        const off = EventsOn("freshclam:event", (event: FreshclamEventPayload) => {
            setFreshclamEvents((events) => [...events, event].slice(-50));
        });
        return off;
    }, []);

    useEffect(() => {
        if (activePage === "quarantine") {
            loadQuarantineRecords();
        }
    }, [activePage]);

    useEffect(() => {
        if (activePage === "logs") {
            loadLogs();
        }
    }, [activePage]);

    useEffect(() => {
        if (activePage === "about") {
            loadAboutInfo();
        }
    }, [activePage]);

    useEffect(() => {
        if (activePage === "settings") {
            loadSystemPermissionStatus();
        }
    }, [activePage]);

    useEffect(() => {
        if (activePage === "results" || activePage === "dashboard") {
            loadResultsJobs();
        }
    }, [activePage]);

    useEffect(() => {
        setResultsPage(1);
    }, [resultsFilter, resultsSearch, resultsJobId]);

    function dismissToast(id: number) {
        setToasts((current) => current.filter((toast) => toast.id !== id));
    }

    function pushToast(tone: ToastTone, title: string, detail?: string) {
        const id = Date.now() + Math.random();
        setToasts((current) => [...current, {id, tone, title, detail}]);
        if (tone !== "pending") {
            window.setTimeout(() => dismissToast(id), 4500);
        }
        return id;
    }

    function toastError(title: string, error: unknown) {
        pushToast("error", title, error instanceof Error ? error.message : undefined);
    }

    function showInfoDialog(title: string, body: string[]) {
        setDialog({title, body});
    }

    function showConfirmDialog(
        title: string,
        body: string[],
        onConfirm: () => void,
        options?: {confirmLabel?: string; danger?: boolean}
    ) {
        setDialog({
            title,
            body,
            confirmLabel: options?.confirmLabel ?? "確認",
            danger: options?.danger,
            onConfirm,
        });
    }

    function refreshStatus() {
        GetAppStatus().then(setStatus);
        loadRuntimeSetup();
    }

    function loadRuntimeSetup() {
        GetRuntimeSetupStatus()
            .then(setRuntimeSetup)
            .catch((error) => {
                toastError("ClamAV runtime 檢測失敗", error);
            });
    }

    function openRuntimeSetupGuide() {
        setForceShowSetup(true);
        loadRuntimeSetup();
    }

    function refreshSettingsView() {
        GetSettings().then((loaded) => {
            setSettings(loaded);
            setSchedulePathsText(loaded.scanSchedule.paths.join("\n"));
        });
        GetAppStatus().then(setStatus);
        loadLoginItemStatus();
        loadSystemPermissionStatus();
        loadRuntimeSetup();
    }

    function loadAboutInfo() {
        GetAboutInfo()
            .then(setAboutInfo)
            .catch((error) => {
                toastError("讀取關於資訊失敗", error);
            });
    }

    function loadLoginItemStatus() {
        GetLoginItemStatus()
            .then(setLoginItemStatus)
            .catch((error) => {
                toastError("讀取 login item 狀態失敗", error);
            });
    }

    function loadSystemPermissionStatus() {
        GetSystemPermissionStatus()
            .then(setSystemPermissions)
            .catch((error) => {
                toastError("讀取 macOS 權限狀態失敗", error);
            });
        CheckNotificationAuthorization()
            .then((authorized) => {
                setNotificationPermission({status: authorized ? "authorized" : "denied"});
            })
            .catch((error) => {
                setNotificationPermission({
                    status: "unknown",
                    message: error instanceof Error ? error.message : "無法讀取 Notifications 權限狀態",
                });
            });
    }

    function runDatabaseUpdate() {
        setFreshclamEvents([]);
        const pendingId = pushToast("pending", "病毒碼更新中", "正在向 ClamAV 伺服器同步病毒碼");
        UpdateDatabase()
            .then(() => {
                dismissToast(pendingId);
                pushToast("success", "病毒碼已更新");
                refreshStatus();
                loadRuntimeSetup();
            })
            .catch((error) => {
                dismissToast(pendingId);
                toastError("病毒碼更新失敗", error);
                refreshStatus();
                loadRuntimeSetup();
            });
    }

    function updateSettings(mutator: (next: main.Settings) => void) {
        if (!settings) {
            return;
        }

        const next = JSON.parse(JSON.stringify(settings)) as main.Settings;
        mutator(next);
        setSettings(next);
        SaveSettings(next)
            .then((saved) => {
                setSettings(saved);
                loadLoginItemStatus();
                pushToast("success", "設定已儲存");
            })
            .catch((error) => {
                toastError("設定儲存失敗", error);
            });
    }

    function removeUserDataCategory(
        key: keyof main.UserDataRemovalOptions,
        title: string,
        body: string[],
        onDone?: () => void
    ) {
        showConfirmDialog(title, body, () => {
            const options: main.UserDataRemovalOptions = {
                removeSettings: false,
                removeScanJobs: false,
                removeScanResults: false,
                removeQuarantine: false,
                removeLogs: false,
            };
            options[key] = true;
            const pendingId = pushToast("pending", `${title}中`);
            RemoveUserData(options)
                .then((result) => {
                    const removed = result?.removed?.length ?? 0;
                    const skipped = result?.skipped?.length ?? 0;
                    const message = `已移除 ${removed} 個項目，略過 ${skipped} 個不存在項目`;
                    dismissToast(pendingId);
                    pushToast("success", title, message);
                    onDone?.();
                })
                .catch((error) => {
                    const message = error instanceof Error ? error.message : `${title}失敗`;
                    dismissToast(pendingId);
                    pushToast("error", `${title}失敗`, message);
                });
        }, {danger: true, confirmLabel: "移除"});
    }

    function scanPaths() {
        return scanPathsText
            .split("\n")
            .map((path) => path.trim())
            .filter((path) => path.length > 0);
    }

    function addScanPreset(preset: {label: string; path: string}) {
        addScanPaths([preset.path]);
        setScanMessage(`已加入${preset.label}：${preset.path}`);
    }

    function addScanPaths(paths: string[]) {
        if (paths.length === 0) {
            return;
        }
        setScanPathsText((current) => {
            const existing = current.split("\n").map((path) => path.trim()).filter((path) => path.length > 0);
            const merged = [...existing];
            for (const path of paths) {
                if (!merged.includes(path)) {
                    merged.push(path);
                }
            }
            return merged.join("\n");
        });
    }

    function selectScanFiles() {
        SelectScanFiles()
            .then(addScanPaths)
            .catch((error) => {
                setScanMessage(error instanceof Error ? error.message : "選擇檔案失敗");
            });
    }

    function selectScanFolder() {
        SelectScanFolder()
            .then((path) => addScanPaths(path ? [path] : []))
            .catch((error) => {
                setScanMessage(error instanceof Error ? error.message : "選擇資料夾失敗");
            });
    }

    function scheduledPathsFromText(value: string) {
        return value
            .split("\n")
            .map((path) => path.trim())
            .filter((path) => path.length > 0);
    }

    function saveSchedulePaths(value: string) {
        updateSettings((next) => {
            next.scanSchedule.paths = scheduledPathsFromText(value);
        });
    }

    function addSchedulePaths(paths: string[]) {
        if (paths.length === 0 || !settings) {
            return;
        }

        const existing = settings.scanSchedule.paths ?? [];
        const merged = [...existing];
        for (const path of paths) {
            if (!merged.includes(path)) {
                merged.push(path);
            }
        }
        setSchedulePathsText(merged.join("\n"));
        updateSettings((next) => {
            next.scanSchedule.paths = merged;
        });
    }

    function selectScheduleFiles() {
        SelectScanFiles()
            .then(addSchedulePaths)
            .catch((error) => {
                toastError("選擇檔案失敗", error);
            });
    }

    function selectScheduleFolder() {
        SelectScanFolder()
            .then((path) => addSchedulePaths(path ? [path] : []))
            .catch((error) => {
                toastError("選擇資料夾失敗", error);
            });
    }

    function openFullDiskAccessSettings() {
        OpenFullDiskAccessSettings()
            .then(() => pushToast("info", "已開啟 Full Disk Access 設定"))
            .catch((error) => {
                toastError("無法開啟 Full Disk Access 設定", error);
            });
    }

    function openNotificationSettings() {
        OpenNotificationSettings()
            .then(() => pushToast("info", "已開啟 Notifications 設定"))
            .catch((error) => {
                toastError("無法開啟 Notifications 設定", error);
            });
    }

    function explainRuntimeInstall() {
        loadRuntimeSetup();
        pushToast("info", "ClamAV 檢測", "ClamAV Desktop 會偵測 Homebrew ClamAV；未通過時會顯示安裝與啟動引導。");
    }

    function showRuntimeRemovalPrompt() {
        const notes = runtimeSetup?.removeNotes ?? [
            "ClamAV Desktop 不移除 Homebrew 或手動安裝的 ClamAV runtime。",
            "若要移除 Homebrew ClamAV，請先關閉 app，再執行 `brew uninstall clamav`。",
        ];
        showInfoDialog("移除 ClamAV runtime", notes);
    }

    function openExternalURL(url: string) {
        BrowserOpenURL(url);
    }

    function startScan() {
        const paths = scanPaths();
        if (paths.length === 0) {
            setScanMessage("請輸入至少一個掃描路徑");
            return;
        }

        const options = {
            recursive: scanRecursive,
            allMatch: false,
        } as main.ScanOptions;

        setScanMessage("掃描中");
        setScanProgress(null);
        setScanResults([]);
        const pendingId = pushToast("pending", "掃描中", `共 ${paths.length} 個路徑`);
        StartScan(paths, options)
            .then((job) => {
                setScanJob(job);
                setScanMessage(formatScanStatus(job.status));
                dismissToast(pendingId);
                pushToast(
                    job.status === "failed" ? "error" : "success",
                    `掃描${formatScanStatus(job.status)}`,
                    `掃描 ${job.scannedFiles} 筆，偵測 ${job.detections} 筆，錯誤 ${job.errors} 筆`
                );
                return LoadScanResults(job.id);
            })
            .then(setScanResults)
            .catch((error) => {
                const message = error instanceof Error ? error.message : "掃描失敗";
                setScanMessage(message);
                dismissToast(pendingId);
                pushToast("error", "掃描失敗", message);
            });
    }

    function cancelScan() {
        const jobId = scanProgress?.jobId || scanJob?.id;
        if (!jobId) {
            return;
        }
        CancelScanJob(jobId).then((canceled) => {
            setScanMessage(canceled ? "取消中" : "找不到執行中的掃描");
        });
    }

    function openScanResult(result: main.ScanResult) {
        OpenScanResultLocation(result).catch((error) => {
            setScanMessage(error instanceof Error ? error.message : "無法開啟位置");
        });
    }

    function quarantineResult(result: main.ScanResult) {
        QuarantineScanResult(result)
            .then(() => {
                setScanMessage("已隔離檔案");
                setScanResults((results) =>
                    results.map((item) => (item.path === result.path ? {...item, status: "quarantined"} : item))
                );
            })
            .catch((error) => {
                setScanMessage(error instanceof Error ? error.message : "隔離失敗");
            });
    }

    function requestPermanentDelete(target: string, action: () => void) {
        if (settings && !settings.actions.confirmPermanentDelete) {
            action();
            return;
        }
        showConfirmDialog(
            "永久刪除",
            ["即將永久刪除此檔案，且無法復原：", target],
            action,
            {danger: true, confirmLabel: "永久刪除"}
        );
    }

    function moveResultToTrash(result: main.ScanResult) {
        MoveScanResultToTrash(result)
            .then(() => {
                setScanMessage("已移到垃圾桶");
                setScanResults((results) =>
                    results.map((item) => (item.path === result.path ? {...item, status: "trashed"} : item))
                );
            })
            .catch((error) => {
                setScanMessage(error instanceof Error ? error.message : "移到垃圾桶失敗");
            });
    }

    function permanentlyDeleteResult(result: main.ScanResult) {
        requestPermanentDelete(result.path, () => {
            PermanentlyDeleteScanResult(result)
                .then(() => {
                    setScanMessage("已永久刪除");
                    pushToast("success", "已永久刪除", result.path);
                    setScanResults((results) =>
                        results.map((item) => (item.path === result.path ? {...item, status: "deleted"} : item))
                    );
                })
                .catch((error) => {
                    const message = error instanceof Error ? error.message : "永久刪除失敗";
                    setScanMessage(message);
                    pushToast("error", "永久刪除失敗", message);
                });
        });
    }

    function loadQuarantineRecords() {
        ListQuarantineRecords()
            .then(setQuarantineRecords)
            .catch((error) => {
                toastError("讀取隔離區失敗", error);
            });
    }

    function openQuarantineRecord(record: main.QuarantineRecord) {
        OpenQuarantineLocation(record).catch((error) => {
            toastError("無法開啟位置", error);
        });
    }

    function restoreQuarantineRecordAction(record: main.QuarantineRecord) {
        RestoreQuarantineRecord(record.id)
            .then((restored) => {
                pushToast("success", "已還原檔案");
                setQuarantineRecords((records) => records.map((item) => (item.id === restored.id ? restored : item)));
            })
            .catch((error) => {
                toastError("還原失敗", error);
            });
    }

    function moveQuarantineRecordToTrashAction(record: main.QuarantineRecord) {
        MoveQuarantineRecordToTrash(record.id)
            .then((trashed) => {
                pushToast("success", "已移到垃圾桶");
                setQuarantineRecords((records) => records.map((item) => (item.id === trashed.id ? trashed : item)));
            })
            .catch((error) => {
                toastError("移到垃圾桶失敗", error);
            });
    }

    function permanentlyDeleteQuarantineRecordAction(record: main.QuarantineRecord) {
        requestPermanentDelete(record.originalPath || record.signature || record.id, () => {
            PermanentlyDeleteQuarantineRecord(record.id)
                .then((deleted) => {
                    pushToast("success", "已永久刪除");
                    setQuarantineRecords((records) => records.map((item) => (item.id === deleted.id ? deleted : item)));
                })
                .catch((error) => {
                    toastError("永久刪除失敗", error);
                });
        });
    }

    function loadLogs() {
        ListAppLogEntries(50)
            .then(setAppLogEntries)
            .catch((error) => {
                toastError("讀取 App Log 失敗", error);
            });
        ListScanJobs()
            .then(setScanJobs)
            .catch((error) => {
                toastError("讀取掃描紀錄失敗", error);
            });
        ReadFreshclamLog(100).then(setFreshclamLogLines);
        ReadClamdLog(100).then(setClamdLogLines);
    }

    function exportDiagnostics() {
        const pendingId = pushToast("pending", "匯出診斷資訊中");
        ExportDiagnostics()
            .then((path) => {
                dismissToast(pendingId);
                pushToast("success", "已匯出診斷報告", path);
            })
            .catch((error) => {
                dismissToast(pendingId);
                toastError("匯出診斷資訊失敗", error);
            });
    }

    function loadResultsJobs() {
        ListScanJobs()
            .then((jobs) => {
                setResultsJobs(jobs);
                if (jobs.length === 0) {
                    setResultsJobId("");
                    setResultsItems([]);
                    return;
                }
                const jobId = jobs.some((job) => job.id === resultsJobId) ? resultsJobId : jobs[0].id;
                setResultsJobId(jobId);
                loadResultsItems(jobId);
            })
            .catch((error) => {
                toastError("讀取掃描紀錄失敗", error);
            });
    }

    function loadResultsItems(jobId: string) {
        LoadScanResults(jobId)
            .then(setResultsItems)
            .catch((error) => {
                toastError("讀取掃描結果失敗", error);
            });
    }

    function selectResultsJob(jobId: string) {
        setResultsJobId(jobId);
        loadResultsItems(jobId);
    }

    function openResultsItem(result: main.ScanResult) {
        OpenScanResultLocation(result).catch((error) => {
            toastError("無法開啟位置", error);
        });
    }

    function quarantineResultsItem(result: main.ScanResult) {
        QuarantineScanResult(result)
            .then(() => {
                pushToast("success", "已隔離檔案");
                setResultsItems((results) =>
                    results.map((item) => (item.path === result.path ? {...item, status: "quarantined"} : item))
                );
            })
            .catch((error) => {
                toastError("隔離失敗", error);
            });
    }

    function moveResultsItemToTrash(result: main.ScanResult) {
        MoveScanResultToTrash(result)
            .then(() => {
                pushToast("success", "已移到垃圾桶");
                setResultsItems((results) =>
                    results.map((item) => (item.path === result.path ? {...item, status: "trashed"} : item))
                );
            })
            .catch((error) => {
                toastError("移到垃圾桶失敗", error);
            });
    }

    function permanentlyDeleteResultsItem(result: main.ScanResult) {
        requestPermanentDelete(result.path, () => {
            PermanentlyDeleteScanResult(result)
                .then(() => {
                    pushToast("success", "已永久刪除", result.path);
                    setResultsItems((results) =>
                        results.map((item) => (item.path === result.path ? {...item, status: "deleted"} : item))
                    );
                })
                .catch((error) => {
                    toastError("永久刪除失敗", error);
                });
        });
    }

    function renderPage() {
        if (activePage === "dashboard") {
            const runtimeWarnings = status?.runtime.warnings ?? [];

            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">系統狀態</p>
                            <div className="sectionTitleRow">
                                <h2>儀表板</h2>
                            </div>
                        </div>
                        <div className="headerActions">
                            <button className="secondaryButton" onClick={runDatabaseUpdate} type="button">更新病毒碼</button>
                            <button className="primaryButton" onClick={explainRuntimeInstall} type="button">檢測 ClamAV</button>
                        </div>
                    </div>
                    <div className="statusGrid">
                        <article className="statusCard">
                            <span>執行環境</span>
                            <strong>{formatRuntimeMode(status?.runtime.mode)}</strong>
                            <p>{formatRuntimeSource(status?.runtime.source)}</p>
                        </article>
                        <article className="statusCard">
                            <span>健康狀態</span>
                            <strong>{formatHealthStatus(status?.health.status)}</strong>
                            <p>執行檔、設定檔、病毒碼資料庫、socket 與 clamd PING</p>
                        </article>
                        <article className="statusCard">
                            <span>掃描引擎</span>
                            <strong>clamd + INSTREAM</strong>
                            <p>正式掃描工作不自動改用 clamscan</p>
                        </article>
                        <article className="statusCard">
                            <span>病毒碼資料庫</span>
                            <strong>{formatDatabaseUpdated(status?.database)}</strong>
                            <p>{formatDatabaseSummary(status?.database)}</p>
                        </article>
                    </div>
                    <div className="runtimeBox">
                        <div>
                            <span>執行環境模式</span>
                            <strong>{formatRuntimeMode(status?.runtime.mode)}</strong>
                        </div>
                        <div>
                            <span>通訊 Socket</span>
                            <code>{status?.runtime.clamdSocket ?? "載入中"}</code>
                        </div>
                        <div>
                            <span>病毒碼資料庫</span>
                            <code>{status?.runtime.databasePath ?? "載入中"}</code>
                        </div>
                        <div>
                            <span>病毒碼狀態</span>
                            <strong>{formatDatabaseUpdated(status?.database)}</strong>
                            <p>{status?.database.error || formatDatabaseSummary(status?.database)}</p>
                        </div>
                    </div>
                    {runtimeWarnings.length > 0 && (
                        <div className="warningList">
                            {runtimeWarnings.map((warning) => (
                                <p key={warning}>{warning}</p>
                            ))}
                        </div>
                    )}
                    <div className="checksTable">
                        {(status?.health.checks ?? []).map((check) => (
                            <div className="checkRow" key={`${check.name}-${check.path}`}>
                                <span className={`checkBadge ${check.status}`}>{formatCheckStatus(check.status)}</span>
                                <div>
                                    <strong>{check.name}</strong>
                                    <p>{check.message}</p>
                                    <code>{check.path || "未設定"}</code>
                                </div>
                            </div>
                        ))}
                    </div>
                    <div className="runtimeBox">
                        <div>
                            <span>最近掃描</span>
                            {resultsJobs.length > 0 ? (
                                <>
                                    <strong>{new Date(resultsJobs[0].startedAt).toLocaleString("zh-TW")}　{formatScanStatus(resultsJobs[0].status)}</strong>
                                    <p>掃描 {resultsJobs[0].scannedFiles} 筆，偵測 {resultsJobs[0].detections} 筆，錯誤 {resultsJobs[0].errors} 筆</p>
                                </>
                            ) : (
                                <strong>尚無掃描紀錄</strong>
                            )}
                        </div>
                        <div>
                            <span>目前掃描工作</span>
                            {scanProgress && (scanProgress.status === "queued" || scanProgress.status === "scanning") ? (
                                <>
                                    <strong>{formatScanStatus(scanProgress.status)}</strong>
                                    <p>已掃描 {scanProgress.scannedFiles} 筆，偵測 {scanProgress.detections} 筆，錯誤 {scanProgress.errors} 筆</p>
                                </>
                            ) : (
                                <strong>目前沒有執行中的掃描</strong>
                            )}
                        </div>
                    </div>
                </section>
            );
        }

        if (activePage === "scan") {
            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">單次掃描</p>
                            <h2>掃描</h2>
                        </div>
                        <div className="headerActions">
                            <div className="btnGroup">
                                <button className="secondaryButton" onClick={selectScanFiles} type="button">選擇檔案</button>
                                <button className="secondaryButton" onClick={selectScanFolder} type="button">選擇資料夾</button>
                                {scanPresets.length > 0 && (
                                    <select
                                        className="presetSelect"
                                        onChange={(event) => {
                                            const preset = scanPresets.find((item) => item.path === event.target.value);
                                            if (preset) {
                                                addScanPreset(preset);
                                            }
                                            event.target.value = "";
                                        }}
                                        value=""
                                    >
                                        <option value="" disabled>選擇目錄</option>
                                        {scanPresets.map((preset) => (
                                            <option key={preset.path} value={preset.path}>{preset.label}</option>
                                        ))}
                                    </select>
                                )}
                            </div>
                            <div className="btnGroup btnGroup--divided">
                                <button className="secondaryButton" disabled={!scanProgress?.jobId} onClick={cancelScan} type="button">取消</button>
                            </div>
                            <div className="btnGroup btnGroup--divided">
                                <button className="primaryButton ctaButton" onClick={startScan} type="button">開始掃描</button>
                            </div>
                        </div>
                    </div>
                    <div className="scanLayout">
                        <div className="scanInputGroup">
                            <label htmlFor="scanPaths">掃描路徑</label>
                            <textarea
                                id="scanPaths"
                                onChange={(event) => setScanPathsText(event.target.value)}
                                placeholder="每行一個完整路徑，例如 /Users/you/Downloads（不支援 ~ 家目錄縮寫）"
                                value={scanPathsText}
                            />
                        </div>
                        <div className="settingsGrid">
                            <label>
                                <span>遞迴掃描資料夾</span>
                                <input checked={scanRecursive} onChange={(event) => setScanRecursive(event.target.checked)} type="checkbox"/>
                            </label>
                        </div>
                    </div>
                    <div className="progressPanel">
                        <div>
                            <span>狀態</span>
                            <strong>{scanProgress ? formatScanStatus(scanProgress.status) : scanMessage || "待命"}</strong>
                        </div>
                        <div>
                            <span>目前檔案</span>
                            <code>{scanProgress?.currentPath || "尚未開始"}</code>
                        </div>
                        <div className="progressGrid">
                            <strong>{scanProgress?.scannedFiles ?? 0}<span>已掃描</span></strong>
                            <strong>{scanProgress?.detections ?? 0}<span>偵測</span></strong>
                            <strong>{scanProgress?.errors ?? 0}<span>警告</span></strong>
                        </div>
                    </div>
                    {scanResults.length > 0 && (
                        <div className="resultsList">
                            {scanResults.map((result) => (
                                <div className="resultRow" key={`${result.path}-${result.status}-${result.signature}`}>
                                    <span className={`checkBadge ${result.status === "clean" || result.status === "quarantined" ? "ok" : result.status === "infected" ? "unhealthy" : "missing"}`}>
                                        {formatResultStatus(result.status)}
                                    </span>
                                <div>
                                    <strong>{result.signature || result.error || result.status}</strong>
                                    <code>{result.path}</code>
                                    <div className="resultActions">
                                        <div className="btnGroup">
                                            <button className="secondaryButton" onClick={() => openScanResult(result)} type="button">前往位置</button>
                                        </div>
                                        {result.status === "infected" && (
                                            <>
                                                <div className="btnGroup btnGroup--divided">
                                                    <button className="actionButton" onClick={() => quarantineResult(result)} type="button">隔離</button>
                                                </div>
                                                <div className="btnGroup btnGroup--divided">
                                                    <button className="dangerSoftButton" onClick={() => moveResultToTrash(result)} type="button">移到垃圾桶</button>
                                                    <button className="dangerButton" onClick={() => permanentlyDeleteResult(result)} type="button">永久刪除</button>
                                                </div>
                                            </>
                                        )}
                                    </div>
                                </div>
                            </div>
                            ))}
                        </div>
                    )}
                </section>
            );
        }

        if (activePage === "schedule") {
            if (!settings) {
                return (
                    <section className="panel emptyPanel">
                        <p className="eyebrow">排程</p>
                        <h2>載入中</h2>
                    </section>
                );
            }

            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">背景工作</p>
                            <div className="sectionTitleRow">
                                <h2>排程</h2>
                            </div>
                        </div>
                    </div>
                    <div className="settingsSections">
                        <section>
                            <h3>排程掃描</h3>
                            <div className="settingsGrid">
                                <label>
                                    <span>啟用</span>
                                    <input checked={settings.scanSchedule.enabled} onChange={(event) => updateSettings((next) => {
                                        next.scanSchedule.enabled = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>頻率</span>
                                    <select onChange={(event) => updateSettings((next) => {
                                        next.scanSchedule.frequency = event.target.value;
                                    })} value={settings.scanSchedule.frequency}>
                                        <option value="daily">每日</option>
                                        <option value="weekly">每週</option>
                                    </select>
                                </label>
                                <label>
                                    <span>時間</span>
                                    <input onChange={(event) => updateSettings((next) => {
                                        next.scanSchedule.timeOfDay = event.target.value;
                                    })} type="time" value={settings.scanSchedule.timeOfDay}/>
                                </label>
                                {settings.scanSchedule.frequency === "weekly" && (
                                    <label>
                                        <span>星期</span>
                                        <select onChange={(event) => updateSettings((next) => {
                                            next.scanSchedule.weekday = Number(event.target.value);
                                        })} value={settings.scanSchedule.weekday}>
                                            {weekdays.map((weekday) => (
                                                <option key={weekday.value} value={weekday.value}>{weekday.label}</option>
                                            ))}
                                        </select>
                                    </label>
                                )}
                            </div>
                            <div className="scanInputGroup">
                                <div className="scanInputGroupHeader">
                                    <label htmlFor="scheduledScanPaths">掃描路徑</label>
                                    <div className="btnGroup">
                                        <button className="secondaryButton" onClick={selectScheduleFiles} type="button">選擇檔案</button>
                                        <button className="secondaryButton" onClick={selectScheduleFolder} type="button">選擇資料夾</button>
                                        <button className="primaryButton" onClick={() => saveSchedulePaths(schedulePathsText)} type="button">儲存路徑</button>
                                    </div>
                                </div>
                                <textarea
                                    id="scheduledScanPaths"
                                    onBlur={(event) => saveSchedulePaths(event.target.value)}
                                    onChange={(event) => setSchedulePathsText(event.target.value)}
                                    placeholder="每行一個完整路徑，例如 /Users/you/Downloads（不支援 ~ 家目錄縮寫）"
                                    value={schedulePathsText}
                                />
                            </div>
                        </section>
                        <section>
                            <h3>病毒碼更新</h3>
                            <div className="settingsStatus">
                                <span>目前狀態</span>
                                <strong>{formatDatabaseUpdated(status?.database)}</strong>
                                <button className="secondaryButton" onClick={runDatabaseUpdate} type="button">立即更新</button>
                            </div>
                            <div className="settingsGrid">
                                <label>
                                    <span>啟用</span>
                                    <input checked={settings.updateSchedule.enabled} onChange={(event) => updateSettings((next) => {
                                        next.updateSchedule.enabled = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>頻率</span>
                                    <select onChange={(event) => updateSettings((next) => {
                                        next.updateSchedule.frequency = event.target.value;
                                    })} value={settings.updateSchedule.frequency}>
                                        <option value="daily">每日</option>
                                    </select>
                                </label>
                                <label>
                                    <span>時間</span>
                                    <input onChange={(event) => updateSettings((next) => {
                                        next.updateSchedule.timeOfDay = event.target.value;
                                    })} type="time" value={settings.updateSchedule.timeOfDay}/>
                                </label>
                            </div>
                        </section>
                        <section>
                            <h3>電源政策</h3>
                            <div className="settingsGrid">
                                <label>
                                    <span>電池供電時執行</span>
                                    <input checked={settings.powerPolicy.runOnBattery} onChange={(event) => updateSettings((next) => {
                                        next.powerPolicy.runOnBattery = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>省電模式下執行</span>
                                    <input checked={settings.powerPolicy.runInLowPowerMode} onChange={(event) => updateSettings((next) => {
                                        next.powerPolicy.runInLowPowerMode = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>延後到充電時執行</span>
                                    <input checked={settings.powerPolicy.deferUntilCharging} onChange={(event) => updateSettings((next) => {
                                        next.powerPolicy.deferUntilCharging = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                            </div>
                        </section>
                    </div>
                </section>
            );
        }

        if (activePage === "results") {
            const filters = ["all", "clean", "infected", "error", "skipped"] as const;
            const query = resultsSearch.trim().toLowerCase();
            const filteredResults = resultsItems.filter((item) => {
                if (resultsFilter !== "all" && item.status !== resultsFilter) {
                    return false;
                }
                if (!query) {
                    return true;
                }
                return (
                    item.path.toLowerCase().includes(query) ||
                    (item.signature ?? "").toLowerCase().includes(query)
                );
            });
            const totalPages = Math.max(1, Math.ceil(filteredResults.length / RESULTS_PAGE_SIZE));
            const currentPage = Math.min(resultsPage, totalPages);
            const pageItems = filteredResults.slice(
                (currentPage - 1) * RESULTS_PAGE_SIZE,
                currentPage * RESULTS_PAGE_SIZE
            );

            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">掃描結果</p>
                            <div className="sectionTitleRow">
                                <h2>結果</h2>
                            </div>
                        </div>
                        <div className="headerActions">
                            <div className="btnGroup">
                                <button className="secondaryButton" onClick={loadResultsJobs} type="button">重新整理</button>
                            </div>
                            <div className="btnGroup btnGroup--divided">
                                <button
                                    className="dangerButton"
                                    onClick={() => removeUserDataCategory(
                                        "removeScanResults",
                                        "清除所有掃描結果",
                                        ["即將刪除所有已儲存的掃描結果紀錄，此操作無法復原。", "（不會影響你磁碟上的實際檔案。）"],
                                        () => { loadResultsJobs(); }
                                    )}
                                    type="button"
                                >
                                    清除所有結果
                                </button>
                            </div>
                        </div>
                    </div>
                    {resultsJobs.length === 0 ? (
                        <p>尚無掃描紀錄。</p>
                    ) : (
                        <>
                            <div className="resultsJobPicker">
                                <label>
                                    <span>掃描紀錄</span>
                                    <select onChange={(event) => selectResultsJob(event.target.value)} value={resultsJobId}>
                                        {resultsJobs.map((job) => (
                                            <option key={job.id} value={job.id}>
                                                {new Date(job.startedAt).toLocaleString("zh-TW")}（{formatScanStatus(job.status)}）
                                            </option>
                                        ))}
                                    </select>
                                </label>
                            </div>
                            <div className="filterBar">
                                {filters.map((filter) => (
                                    <button
                                        className={resultsFilter === filter ? "filterButton active" : "filterButton"}
                                        key={filter}
                                        onClick={() => setResultsFilter(filter)}
                                        type="button"
                                    >
                                        {formatResultFilter(filter)}
                                    </button>
                                ))}
                            </div>
                            <div className="resultsToolbar">
                                <input
                                    className="searchInput"
                                    onChange={(event) => setResultsSearch(event.target.value)}
                                    placeholder="搜尋路徑或病毒碼名稱"
                                    type="search"
                                    value={resultsSearch}
                                />
                            </div>
                            {filteredResults.length === 0 ? (
                                <p>沒有符合篩選條件的結果。</p>
                            ) : (
                                <>
                                    <p className="resultsCount">
                                        共 {filteredResults.length} 筆結果
                                        {totalPages > 1 ? `，顯示第 ${(currentPage - 1) * RESULTS_PAGE_SIZE + 1}–${Math.min(currentPage * RESULTS_PAGE_SIZE, filteredResults.length)} 筆` : ""}
                                    </p>
                                    <div className="resultsList">
                                        {pageItems.map((result) => (
                                            <div className="resultRow" key={`${result.path}-${result.status}-${result.signature}`}>
                                                <span className={`checkBadge ${result.status === "clean" || result.status === "quarantined" ? "ok" : result.status === "infected" ? "unhealthy" : "missing"}`}>
                                                    {formatResultStatus(result.status)}
                                                </span>
                                                <div>
                                                    <strong>{result.signature || formatResultStatus(result.status)}</strong>
                                                    <code>{result.path}</code>
                                                    <p>引擎：{result.engine || "—"}{result.error ? `　錯誤：${result.error}` : ""}</p>
                                                    <div className="resultActions">
                                                        <div className="btnGroup">
                                                            <button className="secondaryButton" onClick={() => openResultsItem(result)} type="button">前往位置</button>
                                                        </div>
                                                        {result.status === "infected" && (
                                                            <>
                                                                <div className="btnGroup btnGroup--divided">
                                                                    <button className="actionButton" onClick={() => quarantineResultsItem(result)} type="button">隔離</button>
                                                                </div>
                                                                <div className="btnGroup btnGroup--divided">
                                                                    <button className="dangerSoftButton" onClick={() => moveResultsItemToTrash(result)} type="button">移到垃圾桶</button>
                                                                    <button className="dangerButton" onClick={() => permanentlyDeleteResultsItem(result)} type="button">永久刪除</button>
                                                                </div>
                                                            </>
                                                        )}
                                                    </div>
                                                </div>
                                            </div>
                                        ))}
                                    </div>
                                    {totalPages > 1 && (
                                        <div className="pagination">
                                            <button
                                                className="secondaryButton"
                                                disabled={currentPage <= 1}
                                                onClick={() => setResultsPage(currentPage - 1)}
                                                type="button"
                                            >
                                                上一頁
                                            </button>
                                            <span>第 {currentPage} / {totalPages} 頁</span>
                                            <button
                                                className="secondaryButton"
                                                disabled={currentPage >= totalPages}
                                                onClick={() => setResultsPage(currentPage + 1)}
                                                type="button"
                                            >
                                                下一頁
                                            </button>
                                        </div>
                                    )}
                                </>
                            )}
                        </>
                    )}
                </section>
            );
        }

        if (activePage === "settings") {
            if (!settings) {
                return (
                    <section className="panel emptyPanel">
                        <p className="eyebrow">設定</p>
                        <h2>載入中</h2>
                    </section>
                );
            }

            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">控制項</p>
                            <div className="sectionTitleRow">
                                <h2>設定</h2>
                            </div>
                        </div>
                    </div>
                    <div className="settingsSections">
                        <section>
                            <h3>背景與登入</h3>
                            <p className="settingHint">「登入時啟動」開啟後，macOS 會在你登入時自動開啟 ClamAV Desktop，讓背景排程持續運作。目前狀態可在「關於」頁查看。</p>
                            <div className="settingsGrid">
                                <label>
                                    <span>背景排程</span>
                                    <input checked={settings.background.enabled} onChange={(event) => updateSettings((next) => {
                                        next.background.enabled = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>登入時啟動</span>
                                    <input checked={settings.login.launchAtLogin} onChange={(event) => updateSettings((next) => {
                                        next.login.launchAtLogin = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label className={settings.background.keepMenuBarIcon ? "" : "disabledLabel"}>
                                    <span>啟動時隱藏視窗</span>
                                    <input checked={settings.background.startHidden && settings.background.keepMenuBarIcon} disabled={!settings.background.keepMenuBarIcon} onChange={(event) => updateSettings((next) => {
                                        next.background.startHidden = event.target.checked;
                                    })} type="checkbox"/>
                                </label>
                                <label>
                                    <span>保留狀態列圖示</span>
                                    <input checked={settings.background.keepMenuBarIcon} onChange={(event) => updateSettings((next) => {
                                        next.background.keepMenuBarIcon = event.target.checked;
                                        if (!event.target.checked) {
                                            next.background.startHidden = false;
                                        }
                                    })} type="checkbox"/>
                                </label>
                            </div>
                            <p className="settingHint">「啟動時隱藏視窗」需先開啟「保留狀態列圖示」，否則啟動後將沒有入口可開啟視窗。</p>
                        </section>
                        <section>
                            <h3>macOS 權限</h3>
                            <div className="settingsStatus">
                                <span>Full Disk Access</span>
                                <strong title={systemPermissions?.fullDiskAccess.message}>{formatPermissionStatus(systemPermissions?.fullDiskAccess.status)}</strong>
                                <button className="secondaryButton" onClick={openFullDiskAccessSettings} type="button">開啟設定</button>
                            </div>
                            <div className="settingsStatus">
                                <span>Notifications</span>
                                <strong title={notificationPermission.message}>{formatPermissionStatus(notificationPermission.status)}</strong>
                                <button className="secondaryButton" onClick={openNotificationSettings} type="button">開啟設定</button>
                            </div>
                            <button className="secondaryButton" onClick={loadSystemPermissionStatus} type="button">重新檢測</button>
                        </section>
                        <section>
                            <h3>ClamAV runtime</h3>
                            <div className="settingsStatus">
                                <span>安裝狀態</span>
                                <strong>{runtimeSetup?.ready ? "可使用" : "需處理"}</strong>
                                <button className="secondaryButton" onClick={loadRuntimeSetup} type="button">重新檢測</button>
                            </div>
                            <div className="settingsStatus">
                                <span>移除方式</span>
                                <strong>提示移除</strong>
                                <button className="secondaryButton" onClick={showRuntimeRemovalPrompt} type="button">顯示提示</button>
                            </div>
                        </section>
                        <section>
                            <h3>重置設定</h3>
                            <p className="settingHint">將 ClamAV Desktop 的設定檔還原為預設值。</p>
                            <button
                                className="dangerButton"
                                onClick={() => removeUserDataCategory(
                                    "removeSettings",
                                    "重置設定檔",
                                    ["即將把設定檔還原為預設值，此操作無法復原。"],
                                    refreshSettingsView
                                )}
                                type="button"
                            >
                                重置設定檔
                            </button>
                        </section>
                    </div>
                </section>
            );
        }

        if (activePage === "logs") {
            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">紀錄</p>
                            <div className="sectionTitleRow">
                                <h2>紀錄</h2>
                            </div>
                        </div>
                        <div className="headerActions">
                            <div className="btnGroup">
                                <button className="secondaryButton" onClick={loadLogs} type="button">重新整理</button>
                                <button className="secondaryButton" onClick={exportDiagnostics} type="button">匯出診斷資訊</button>
                                <button className="secondaryButton" onClick={runDatabaseUpdate} type="button">更新病毒碼</button>
                            </div>
                            <div className="btnGroup btnGroup--divided">
                                <button
                                    className="dangerButton"
                                    onClick={() => removeUserDataCategory(
                                        "removeScanJobs",
                                        "清除掃描工作紀錄",
                                        ["即將刪除所有掃描工作紀錄，此操作無法復原。", "（不會影響你磁碟上的實際檔案。）"],
                                        () => { loadLogs(); }
                                    )}
                                    type="button"
                                >
                                    清除掃描工作
                                </button>
                                <button
                                    className="dangerButton"
                                    onClick={() => removeUserDataCategory(
                                        "removeLogs",
                                        "清除 App log",
                                        ["即將刪除 App log，此操作無法復原。"],
                                        () => { loadLogs(); }
                                    )}
                                    type="button"
                                >
                                    清除 App log
                                </button>
                            </div>
                        </div>
                    </div>
                    <div className="runtimeBox">
                        <div>
                            <span>病毒碼狀態</span>
                            <strong>{formatDatabaseUpdated(status?.database)}</strong>
                            <p>{formatDatabaseSummary(status?.database)}</p>
                        </div>
                        <div>
                            <span>病毒碼資料庫路徑</span>
                            <code>{status?.database.path || status?.runtime.databasePath || "未設定"}</code>
                        </div>
                    </div>
                    {freshclamEvents.length > 0 && (
                        <div className="logList">
                            {freshclamEvents.map((event, index) => (
                                <div className="logRow" key={`${event.at}-${index}`}>
                                    <span className={`checkBadge ${event.stream === "stderr" ? "unhealthy" : "ok"}`}>
                                        {event.stream}
                                    </span>
                                    <code>{event.message}</code>
                                </div>
                            ))}
                        </div>
                    )}
                    <div className="runtimeBox">
                        <h3>App Log</h3>
                        {appLogEntries.length === 0 ? (
                            <p>尚無紀錄。</p>
                        ) : (
                            <div className="logList">
                                {appLogEntries.map((entry, index) => (
                                    <div className="logRow" key={`${entry.at}-${index}`}>
                                        <span className={`checkBadge ${entry.level === "error" ? "unhealthy" : "ok"}`}>
                                            {entry.level}
                                        </span>
                                        <code>
                                            {new Date(entry.at).toLocaleString("zh-TW")}　{entry.message}
                                            {entry.source ? `　@ ${entry.source}` : ""}
                                        </code>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                    <div className="runtimeBox">
                        <h3>掃描紀錄</h3>
                        {scanJobs.length === 0 ? (
                            <p>尚無掃描紀錄。</p>
                        ) : (
                            <div className="resultsList">
                                {scanJobs.map((job) => (
                                    <div className="resultRow" key={job.id}>
                                        <span className={`checkBadge ${job.status === "completed" ? "ok" : job.status === "completed-with-warnings" || job.status === "failed" ? "unhealthy" : "missing"}`}>
                                            {formatScanStatus(job.status)}
                                        </span>
                                        <div>
                                            <strong>{new Date(job.startedAt).toLocaleString("zh-TW")}</strong>
                                            <p>掃描 {job.scannedFiles} 筆，偵測 {job.detections} 筆，錯誤 {job.errors} 筆</p>
                                            <code>{job.paths.join("、")}</code>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                    <div className="runtimeBox logSection">
                        <h3>freshclam.log</h3>
                        <p className="settingHint">記錄病毒碼更新流程，可用來確認更新是否成功、網路或權限是否異常。</p>
                        {freshclamLogLines.length === 0 ? (
                            <p>尚無紀錄（共用執行環境尚未安裝或無法讀取）。</p>
                        ) : (
                            <div className="logList">
                                {freshclamLogLines.map((line, index) => (
                                    <div className="logRow" key={index}>
                                        <span className="checkBadge ok">freshclam</span>
                                        <code>{line}</code>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                    <div className="runtimeBox logSection">
                        <h3>clamd.log</h3>
                        <p className="settingHint">記錄 clamd 背景服務狀態，可用來確認 daemon 是否啟動、socket 或病毒碼載入是否正常。</p>
                        {clamdLogLines.length === 0 ? (
                            <p>尚無紀錄（共用執行環境尚未安裝或無法讀取）。</p>
                        ) : (
                            <div className="logList">
                                {clamdLogLines.map((line, index) => (
                                    <div className="logRow" key={index}>
                                        <span className="checkBadge ok">clamd</span>
                                        <code>{line}</code>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </section>
            );
        }

        if (activePage === "quarantine") {
            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">隔離區</p>
                            <div className="sectionTitleRow">
                                <h2>隔離區</h2>
                            </div>
                        </div>
                        <div className="headerActions">
                            <div className="btnGroup">
                                <button className="secondaryButton" onClick={loadQuarantineRecords} type="button">重新整理</button>
                            </div>
                            <div className="btnGroup btnGroup--divided">
                                <button
                                    className="dangerButton"
                                    onClick={() => removeUserDataCategory(
                                        "removeQuarantine",
                                        "清空隔離區",
                                        ["即將永久移除隔離區內的所有已隔離檔案與紀錄，此操作無法復原。"],
                                        () => { loadQuarantineRecords(); }
                                    )}
                                    type="button"
                                >
                                    清空隔離區
                                </button>
                            </div>
                        </div>
                    </div>
                    {quarantineRecords.length === 0 ? (
                        <p>尚無隔離項目。</p>
                    ) : (
                        <div className="resultsList">
                            {quarantineRecords.map((record) => (
                                <div className="resultRow" key={record.id}>
                                    <span className={`checkBadge ${record.status === "quarantined" ? "ok" : "missing"}`}>
                                        {formatQuarantineStatus(record.status)}
                                    </span>
                                    <div>
                                        <strong>{record.signature || "未知病毒碼"}</strong>
                                        <code>{record.originalPath}</code>
                                        <p>偵測時間：{new Date(record.detectedAt).toLocaleString("zh-TW")}</p>
                                        <div className="resultActions">
                                            {(record.status === "quarantined" || record.status === "restored") && (
                                                <div className="btnGroup">
                                                    <button className="secondaryButton" onClick={() => openQuarantineRecord(record)} type="button">前往位置</button>
                                                </div>
                                            )}
                                            {record.status === "quarantined" && (
                                                <>
                                                    <div className="btnGroup btnGroup--divided">
                                                        <button className="actionButton" onClick={() => restoreQuarantineRecordAction(record)} type="button">還原</button>
                                                    </div>
                                                    <div className="btnGroup btnGroup--divided">
                                                        <button className="dangerSoftButton" onClick={() => moveQuarantineRecordToTrashAction(record)} type="button">移到垃圾桶</button>
                                                        <button className="dangerButton" onClick={() => permanentlyDeleteQuarantineRecordAction(record)} type="button">永久刪除</button>
                                                    </div>
                                                </>
                                            )}
                                        </div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </section>
            );
        }

        if (activePage === "about") {
            if (!aboutInfo) {
                return (
                    <section className="panel emptyPanel">
                        <p className="eyebrow">關於</p>
                        <h2>載入中</h2>
                    </section>
                );
            }

            const pathRows = [
                {label: "clamscan", value: aboutInfo.paths.clamScan},
                {label: "freshclam", value: aboutInfo.paths.freshclam},
                {label: "clamd", value: aboutInfo.paths.clamd},
                {label: "clamd socket", value: aboutInfo.paths.clamdSocket},
                {label: "runtime config", value: aboutInfo.paths.runtimeConfig},
                {label: "freshclam config", value: aboutInfo.paths.freshclamConfig},
                {label: "病毒碼資料庫", value: aboutInfo.paths.database},
                {label: "隔離區", value: aboutInfo.paths.quarantine},
                {label: "設定檔", value: aboutInfo.paths.settings},
                {label: "Log", value: aboutInfo.paths.logs},
            ];

            return (
                <section className="panel">
                    <div className="sectionHeader">
                        <div>
                            <p className="eyebrow">應用程式資訊</p>
                            <h2>關於</h2>
                        </div>
                        <div className="headerActions">
                            <div className="btnGroup">
                                <button className="secondaryButton" onClick={() => { loadAboutInfo(); loadLoginItemStatus(); loadRuntimeSetup(); }} type="button">重新整理</button>
                                <button className="secondaryButton" onClick={openRuntimeSetupGuide} type="button">安裝／啟動引導</button>
                                <button className="secondaryButton" onClick={() => openExternalURL(aboutInfo.officialUrl)} type="button">ClamAV 官網</button>
                                <button className="secondaryButton" onClick={() => openExternalURL(aboutInfo.githubUrl)} type="button">GitHub Repo</button>
                            </div>
                        </div>
                    </div>
                    <div className="aboutGrid">
                        <article className="statusCard">
                            <span>版本</span>
                            <strong>{aboutInfo.version}</strong>
                            <p>ClamAV Desktop</p>
                        </article>
                        <article className="statusCard">
                            <span>這台電腦</span>
                            <strong>{aboutInfo.computer.hostname || "未知主機"}</strong>
                            <p>{aboutInfo.computer.os}/{aboutInfo.computer.arch}</p>
                        </article>
                        <article className="statusCard">
                            <span>執行環境模式</span>
                            <strong>{formatRuntimeMode(aboutInfo.runtime.mode)}</strong>
                            <p>{formatRuntimeSource(aboutInfo.runtime.source)}</p>
                        </article>
                    </div>
                    <div className="runtimeBox">
                        <div>
                            <span>Home 目錄</span>
                            <code>{aboutInfo.computer.homeDir}</code>
                        </div>
                        <div>
                            <span>病毒碼狀態</span>
                            <strong>{formatDatabaseUpdated(aboutInfo.database)}</strong>
                            <p>{formatDatabaseSummary(aboutInfo.database)}</p>
                        </div>
                    </div>
                    <div className="aboutSection">
                        <h3>背景與登入狀態</h3>
                        <div className="pathTable">
                            <div className="pathRow">
                                <span>登入時自動啟動</span>
                                <strong>{formatLoginItemStatus(loginItemStatus)}</strong>
                            </div>
                            <div className="pathRow">
                                <span>背景排程</span>
                                <strong>{settings ? (settings.background.enabled ? "啟用" : "停用") : "載入中"}</strong>
                            </div>
                            <div className="pathRow">
                                <span>登入時啟動</span>
                                <strong>{settings ? (settings.login.launchAtLogin ? "啟用" : "停用") : "載入中"}</strong>
                            </div>
                            <div className="pathRow">
                                <span>啟動時隱藏視窗</span>
                                <strong>{settings ? (settings.background.startHidden ? "啟用" : "停用") : "載入中"}</strong>
                            </div>
                            <div className="pathRow">
                                <span>保留狀態列圖示</span>
                                <strong>{settings ? (settings.background.keepMenuBarIcon ? "啟用" : "停用") : "載入中"}</strong>
                            </div>
                            <div className="pathRow">
                                <span>ClamAV runtime</span>
                                <strong>{runtimeSetup ? (runtimeSetup.ready ? "可使用" : "需處理") : "檢查中"}</strong>
                            </div>
                        </div>
                        <p className="settingHint">
                            避免排程不執行：保持背景排程、登入時啟動、保留狀態列圖示開啟，並確認排程掃描已啟用且有掃描路徑。只關閉視窗時排程仍會執行；從狀態列結束 app 或電腦關機時，排程不會執行。
                        </p>
                    </div>
                    <div className="aboutSection">
                        <h3>ClamAV 路徑</h3>
                        <div className="pathTable">
                            {pathRows.map((row) => (
                                <div className="pathRow" key={row.label}>
                                    <span>{row.label}</span>
                                    <code>{row.value || "未設定"}</code>
                                </div>
                            ))}
                        </div>
                    </div>
                    <div className="aboutSection">
                        <h3>CLI 常用指令</h3>
                        <p className="settingHint">有需要時，可複製底下指令到 Terminal 手動檢查版本、更新病毒碼、掃描資料夾或建立 macOS 排程。</p>
                        <div className="pathTable">
                            {aboutInfo.commands.map((command) => (
                                <div className="pathRow" key={command.label}>
                                    <span>{command.label}</span>
                                    <code>{command.command}</code>
                                </div>
                            ))}
                        </div>
                    </div>
                    <div className="aboutSection">
                        <h3>功能清單</h3>
                        <div className="resultsList">
                            {aboutInfo.features.map((feature) => (
                                <div className="resultRow" key={feature.name}>
                                    <span className={`checkBadge ${featureStatusClass(feature.status)}`}>{feature.status}</span>
                                    <div>
                                        <strong>{feature.name}</strong>
                                        <p>{feature.note}</p>
                                    </div>
                                </div>
                            ))}
                        </div>
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
                    <img alt="" className="brandIcon" src={appIcon}/>
                    <div>
                        <strong>ClamAV Desktop</strong>
                        <span>macOS 掃描工具</span>
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
                {renderPage()}
                {runtimeSetup && (runtimeSetup.blocking || forceShowSetup) && (
                    <div className="setupOverlay" role="dialog" aria-modal="true">
                        <div className="setupDialog">
                            <div className="setupHeader">
                                <img alt="" className="brandIcon" src={appIcon}/>
                                <div>
                                    <p className="eyebrow">ClamAV 檢測</p>
                                    <h2>需要完成 ClamAV 安裝或啟動</h2>
                                </div>
                                {!runtimeSetup.blocking && (
                                    <button className="setupClose" onClick={() => setForceShowSetup(false)} type="button" aria-label="關閉">×</button>
                                )}
                            </div>
                            <p className="setupMessage">{runtimeSetup.message}</p>
                            <div className="checksTable">
                                {runtimeSetup.health.checks.map((check) => (
                                    <div className="checkRow" key={`${check.name}-${check.path}`}>
                                        <span className={`checkBadge ${check.status}`}>{formatCheckStatus(check.status)}</span>
                                        <div>
                                            <strong>{check.name}</strong>
                                            <p>{check.message}</p>
                                            <code>{check.path || "未設定"}</code>
                                        </div>
                                    </div>
                                ))}
                            </div>
                            <div className="setupSteps">
                                {runtimeSetup.steps.map((step, index) => (
                                    <section className="setupStep" key={`${step.title}-${index}`}>
                                        <div>
                                            <span>{index + 1}</span>
                                            <strong>{step.title}</strong>
                                        </div>
                                        <p>{step.detail}</p>
                                        {step.command && <code>{step.command}</code>}
                                        {step.url && (
                                            <button className="secondaryButton" onClick={() => openExternalURL(step.url)} type="button">開啟連結</button>
                                        )}
                                    </section>
                                ))}
                            </div>
                            <div className="setupActions">
                                {!runtimeSetup.blocking && (
                                    <button className="secondaryButton" onClick={() => setForceShowSetup(false)} type="button">關閉</button>
                                )}
                                <button className="secondaryButton" onClick={runDatabaseUpdate} type="button">更新病毒碼</button>
                                <button className="primaryButton" onClick={loadRuntimeSetup} type="button">重新檢測</button>
                            </div>
                        </div>
                    </div>
                )}
                {dialog && (
                    <div className="dialogOverlay" role="dialog" aria-modal="true">
                        <div className="dialogBox">
                            <h3>{dialog.title}</h3>
                            <div className="dialogBody">
                                {dialog.body.map((line, index) => (
                                    <span key={index}>{line}</span>
                                ))}
                            </div>
                            <div className="dialogActions">
                                {dialog.onConfirm ? (
                                    <>
                                        <button className="secondaryButton" onClick={() => setDialog(null)} type="button">取消</button>
                                        <button
                                            className={dialog.danger ? "dangerButton" : "primaryButton"}
                                            onClick={() => {
                                                const confirm = dialog.onConfirm;
                                                setDialog(null);
                                                confirm?.();
                                            }}
                                            type="button"
                                        >
                                            {dialog.confirmLabel}
                                        </button>
                                    </>
                                ) : (
                                    <button className="primaryButton" onClick={() => setDialog(null)} type="button">知道了</button>
                                )}
                            </div>
                        </div>
                    </div>
                )}
            </main>
            {toasts.length > 0 && (
                <div className="toastStack">
                    {toasts.map((toast) => (
                        <div className={`toast ${toast.tone}`} key={toast.id}>
                            <div>
                                <strong>{toast.title}</strong>
                                {toast.detail && <p>{toast.detail}</p>}
                            </div>
                            <button onClick={() => dismissToast(toast.id)} type="button" aria-label="關閉">×</button>
                        </div>
                    ))}
                </div>
            )}
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

    if (mode === "external") {
        return "外部執行環境";
    }

    if (mode === "manual") {
        return "手動指定";
    }

    if (mode === "per-user") {
        return "使用者個別";
    }

    return mode;
}

function formatRuntimeSource(source?: string) {
    if (!source) {
        return "檢查中";
    }

    if (source === "app-managed-system-shared") {
        return "App 管理的系統共用";
    }

    if (source === "per-user-runtime") {
        return "使用者個別執行環境";
    }

    if (source === "manual") {
        return "手動指定執行環境";
    }

    if (source === "homebrew-arm64") {
        return "Homebrew Apple Silicon 路徑";
    }

    if (source === "homebrew-x86_64") {
        return "Homebrew Intel 路徑";
    }

    if (source === "official-pkg") {
        return "ClamAV 官方 pkg 路徑";
    }

    return source;
}

function featureStatusClass(status: string) {
    if (status === "可用") {
        return "ok";
    }
    if (status === "部分完成") {
        return "missing";
    }
    return "unhealthy";
}

function formatHealthStatus(status?: string) {
    if (!status) {
        return "檢查中";
    }

    if (status === "healthy") {
        return "正常";
    }

    if (status === "repair-required") {
        return "需要修復";
    }

    if (status === "missing") {
        return "尚未安裝";
    }

    return status;
}

function formatCheckStatus(status: string) {
    if (status === "ok") {
        return "正常";
    }

    if (status === "missing") {
        return "缺少";
    }

    if (status === "unhealthy") {
        return "需修復";
    }

    return status;
}

function formatDatabaseUpdated(database?: main.DatabaseStatus) {
    if (!database) {
        return "檢查中";
    }

    if (database.error) {
        return "需要處理";
    }

    if (!database.lastUpdated || database.lastUpdated.startsWith("0001-")) {
        return "尚未更新";
    }

    return new Date(database.lastUpdated).toLocaleString("zh-TW");
}

function formatDatabaseSummary(database?: main.DatabaseStatus) {
	if (!database) {
		return "正在讀取病毒碼狀態";
	}

    if (database.error) {
        return database.error;
    }

    const parts = [];
    if (database.version) {
        parts.push(`版本 ${database.version}`);
    }
    if (database.signatures > 0) {
        parts.push(`${database.signatures.toLocaleString("zh-TW")} 筆簽章`);
    }
	return parts.length > 0 ? parts.join("，") : "尚無更新紀錄";
}

function formatLoginItemStatus(status?: main.LoginItemStatus | null) {
	if (!status) {
		return "載入中";
	}
	if (status.error) {
		return "讀取失敗";
	}
	return status.enabled ? "已註冊" : "未註冊";
}

function formatPermissionStatus(status?: string) {
	if (status === "authorized") {
		return "已授權";
	}
	if (status === "denied") {
		return "未授權";
	}
	if (status === "unknown") {
		return "無法判定";
	}
	return "檢查中";
}

function formatScanStatus(status: string) {
    if (status === "queued") {
        return "等待中";
    }
    if (status === "scanning") {
        return "掃描中";
    }
    if (status === "completed") {
        return "已完成";
    }
    if (status === "completed-with-warnings") {
        return "完成但有警告";
    }
    if (status === "canceled") {
        return "已取消";
    }
    if (status === "failed") {
        return "失敗";
    }
    return status;
}

function formatResultStatus(status: string) {
    if (status === "clean") {
        return "乾淨";
    }
    if (status === "infected") {
        return "感染";
    }
    if (status === "quarantined") {
        return "已隔離";
    }
    if (status === "trashed") {
        return "已移到垃圾桶";
    }
    if (status === "deleted") {
        return "已永久刪除";
    }
    if (status === "skipped") {
        return "略過";
    }
    if (status === "error") {
        return "錯誤";
    }
    return status;
}

function formatResultFilter(filter: string) {
    if (filter === "all") {
        return "全部";
    }
    return formatResultStatus(filter);
}

function formatQuarantineStatus(status: string) {
    if (status === "quarantined") {
        return "隔離中";
    }
    if (status === "restored") {
        return "已還原";
    }
    if (status === "trashed") {
        return "已移到垃圾桶";
    }
    if (status === "deleted") {
        return "已永久刪除";
    }
    return status;
}

export default App;
