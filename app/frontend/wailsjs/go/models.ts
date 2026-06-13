export namespace main {

	export class ActionSettings {
	    confirmPermanentDelete: boolean;

	    static createFrom(source: any = {}) {
	        return new ActionSettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.confirmPermanentDelete = source["confirmPermanentDelete"];
	    }
	}
	export class DatabaseStatus {
	    path: string;
	    // Go type: time
	    lastUpdated: any;
	    version: string;
	    signatures: number;
	    error: string;

	    static createFrom(source: any = {}) {
	        return new DatabaseStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.lastUpdated = this.convertValues(source["lastUpdated"], null);
	        this.version = source["version"];
	        this.signatures = source["signatures"];
	        this.error = source["error"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeCheck {
	    name: string;
	    path: string;
	    status: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new RuntimeCheck(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	export class RuntimeHealth {
	    status: string;
	    checks: RuntimeCheck[];

	    static createFrom(source: any = {}) {
	        return new RuntimeHealth(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.checks = this.convertValues(source["checks"], RuntimeCheck);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeProfile {
	    mode: string;
	    clamScanPath: string;
	    freshclamPath: string;
	    clamdPath: string;
	    clamdSocket: string;
	    databasePath: string;
	    configPath: string;
	    source: string;
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new RuntimeProfile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.clamScanPath = source["clamScanPath"];
	        this.freshclamPath = source["freshclamPath"];
	        this.clamdPath = source["clamdPath"];
	        this.clamdSocket = source["clamdSocket"];
	        this.databasePath = source["databasePath"];
	        this.configPath = source["configPath"];
	        this.source = source["source"];
	        this.warnings = source["warnings"];
	    }
	}
	export class ComputerInfo {
	    hostname: string;
	    homeDir: string;
	    os: string;
	    arch: string;

	    static createFrom(source: any = {}) {
	        return new ComputerInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.homeDir = source["homeDir"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	    }
	}
	export class AboutPaths {
	    clamScan: string;
	    freshclam: string;
	    clamd: string;
	    clamdSocket: string;
	    runtimeConfig: string;
	    freshclamConfig: string;
	    database: string;
	    quarantine: string;
	    settings: string;
	    logs: string;

	    static createFrom(source: any = {}) {
	        return new AboutPaths(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clamScan = source["clamScan"];
	        this.freshclam = source["freshclam"];
	        this.clamd = source["clamd"];
	        this.clamdSocket = source["clamdSocket"];
	        this.runtimeConfig = source["runtimeConfig"];
	        this.freshclamConfig = source["freshclamConfig"];
	        this.database = source["database"];
	        this.quarantine = source["quarantine"];
	        this.settings = source["settings"];
	        this.logs = source["logs"];
	    }
	}
	export class CommandInfo {
	    label: string;
	    command: string;

	    static createFrom(source: any = {}) {
	        return new CommandInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.command = source["command"];
	    }
	}
	export class FeatureStatus {
	    name: string;
	    status: string;
	    note: string;

	    static createFrom(source: any = {}) {
	        return new FeatureStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.note = source["note"];
	    }
	}
	export class AboutInfo {
	    version: string;
	    computer: ComputerInfo;
	    runtime: RuntimeProfile;
	    database: DatabaseStatus;
	    paths: AboutPaths;
	    commands: CommandInfo[];
	    officialUrl: string;
	    githubUrl: string;
	    features: FeatureStatus[];

	    static createFrom(source: any = {}) {
	        return new AboutInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.computer = this.convertValues(source["computer"], ComputerInfo);
	        this.runtime = this.convertValues(source["runtime"], RuntimeProfile);
	        this.database = this.convertValues(source["database"], DatabaseStatus);
	        this.paths = this.convertValues(source["paths"], AboutPaths);
	        this.commands = this.convertValues(source["commands"], CommandInfo);
	        this.officialUrl = source["officialUrl"];
	        this.githubUrl = source["githubUrl"];
	        this.features = this.convertValues(source["features"], FeatureStatus);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeSetupStep {
	    title: string;
	    detail: string;
	    command: string;
	    url: string;

	    static createFrom(source: any = {}) {
	        return new RuntimeSetupStep(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.command = source["command"];
	        this.url = source["url"];
	    }
	}
	export class RuntimeSetupStatus {
	    ready: boolean;
	    blocking: boolean;
	    message: string;
	    profile: RuntimeProfile;
	    health: RuntimeHealth;
	    steps: RuntimeSetupStep[];
	    removeNotes: string[];

	    static createFrom(source: any = {}) {
	        return new RuntimeSetupStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.blocking = source["blocking"];
	        this.message = source["message"];
	        this.profile = this.convertValues(source["profile"], RuntimeProfile);
	        this.health = this.convertValues(source["health"], RuntimeHealth);
	        this.steps = this.convertValues(source["steps"], RuntimeSetupStep);
	        this.removeNotes = source["removeNotes"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppStatus {
	    runtime: RuntimeProfile;
	    health: RuntimeHealth;
	    database: DatabaseStatus;
	    pages: string[];

	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runtime = this.convertValues(source["runtime"], RuntimeProfile);
	        this.health = this.convertValues(source["health"], RuntimeHealth);
	        this.database = this.convertValues(source["database"], DatabaseStatus);
	        this.pages = source["pages"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Background {
	    enabled: boolean;
	    startHidden: boolean;
	    keepMenuBarIcon: boolean;

	    static createFrom(source: any = {}) {
	        return new Background(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.startHidden = source["startHidden"];
	        this.keepMenuBarIcon = source["keepMenuBarIcon"];
	    }
	}

	export class LoginSettings {
	    launchAtLogin: boolean;

	    static createFrom(source: any = {}) {
	        return new LoginSettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.launchAtLogin = source["launchAtLogin"];
	    }
	}
	export class LoginItemStatus {
	    enabled: boolean;
	    path: string;
	    error: string;
	    method: string;

	    static createFrom(source: any = {}) {
	        return new LoginItemStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.path = source["path"];
	        this.error = source["error"];
	        this.method = source["method"];
	    }
	}
	export class PowerPolicy {
	    runOnBattery: boolean;
	    runInLowPowerMode: boolean;
	    deferUntilCharging: boolean;

	    static createFrom(source: any = {}) {
	        return new PowerPolicy(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runOnBattery = source["runOnBattery"];
	        this.runInLowPowerMode = source["runInLowPowerMode"];
	        this.deferUntilCharging = source["deferUntilCharging"];
	    }
	}
	export class QuarantineRecord {
	    id: string;
	    originalPath: string;
	    quarantinePath: string;
	    signature: string;
	    // Go type: time
	    detectedAt: any;
	    sha256: string;
	    status: string;
	    // Go type: time
	    restoredAt?: any;

	    static createFrom(source: any = {}) {
	        return new QuarantineRecord(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.originalPath = source["originalPath"];
	        this.quarantinePath = source["quarantinePath"];
	        this.signature = source["signature"];
	        this.detectedAt = this.convertValues(source["detectedAt"], null);
	        this.sha256 = source["sha256"];
	        this.status = source["status"];
	        this.restoredAt = this.convertValues(source["restoredAt"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}



	export class ScanOptions {
	    recursive: boolean;
	    allMatch: boolean;

	    static createFrom(source: any = {}) {
	        return new ScanOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recursive = source["recursive"];
	        this.allMatch = source["allMatch"];
	    }
	}
	export class ScanJob {
	    id: string;
	    paths: string[];
	    options: ScanOptions;
	    status: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    endedAt?: any;
	    scannedFiles: number;
	    detections: number;
	    errors: number;

	    static createFrom(source: any = {}) {
	        return new ScanJob(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.paths = source["paths"];
	        this.options = this.convertValues(source["options"], ScanOptions);
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.endedAt = this.convertValues(source["endedAt"], null);
	        this.scannedFiles = source["scannedFiles"];
	        this.detections = source["detections"];
	        this.errors = source["errors"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class LogEntry {
	    // Go type: time
	    at: any;
	    level: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = this.convertValues(source["at"], null);
	        this.level = source["level"];
	        this.message = source["message"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ScanResult {
	    path: string;
	    status: string;
	    signature: string;
	    engine: string;
	    error: string;

	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.signature = source["signature"];
	        this.engine = source["engine"];
	        this.error = source["error"];
	    }
	}
	export class ScanSchedule {
	    enabled: boolean;
	    frequency: string;
	    timeOfDay: string;
	    weekday: number;
	    paths: string[];

	    static createFrom(source: any = {}) {
	        return new ScanSchedule(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.frequency = source["frequency"];
	        this.timeOfDay = source["timeOfDay"];
	        this.weekday = source["weekday"];
	        this.paths = source["paths"];
	    }
	}
	export class UpdateSchedule {
	    enabled: boolean;
	    frequency: string;
	    timeOfDay: string;

	    static createFrom(source: any = {}) {
	        return new UpdateSchedule(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.frequency = source["frequency"];
	        this.timeOfDay = source["timeOfDay"];
	    }
	}
	export class UserDataRemovalOptions {
	    removeSettings: boolean;
	    removeScanJobs: boolean;
	    removeScanResults: boolean;
	    removeQuarantine: boolean;
	    removeLogs: boolean;

	    static createFrom(source: any = {}) {
	        return new UserDataRemovalOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removeSettings = source["removeSettings"];
	        this.removeScanJobs = source["removeScanJobs"];
	        this.removeScanResults = source["removeScanResults"];
	        this.removeQuarantine = source["removeQuarantine"];
	        this.removeLogs = source["removeLogs"];
	    }
	}
	export class UserDataRemovalResult {
	    removed: string[];
	    skipped: string[];

	    static createFrom(source: any = {}) {
	        return new UserDataRemovalResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removed = source["removed"];
	        this.skipped = source["skipped"];
	    }
	}
	export class Settings {
	    schemaVersion: number;
	    runtimeMode: string;
	    scanSchedule: ScanSchedule;
	    updateSchedule: UpdateSchedule;
	    powerPolicy: PowerPolicy;
	    background: Background;
	    login: LoginSettings;
	    actions: ActionSettings;

	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.runtimeMode = source["runtimeMode"];
	        this.scanSchedule = this.convertValues(source["scanSchedule"], ScanSchedule);
	        this.updateSchedule = this.convertValues(source["updateSchedule"], UpdateSchedule);
	        this.powerPolicy = this.convertValues(source["powerPolicy"], PowerPolicy);
	        this.background = this.convertValues(source["background"], Background);
	        this.login = this.convertValues(source["login"], LoginSettings);
	        this.actions = this.convertValues(source["actions"], ActionSettings);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
