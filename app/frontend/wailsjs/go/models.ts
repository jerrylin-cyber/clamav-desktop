export namespace main {
	
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
	export class AppStatus {
	    runtime: RuntimeProfile;
	    pages: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runtime = this.convertValues(source["runtime"], RuntimeProfile);
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

}

