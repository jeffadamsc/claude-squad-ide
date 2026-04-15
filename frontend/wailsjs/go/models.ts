export namespace app {
	
	export class AppConfig {
	    DefaultProgram: string;
	    AutoYes: boolean;
	    BranchPrefix: string;
	    Profiles: config.Profile[];
	    DefaultWorkDir: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DefaultProgram = source["DefaultProgram"];
	        this.AutoYes = source["AutoYes"];
	        this.BranchPrefix = source["BranchPrefix"];
	        this.Profiles = this.convertValues(source["Profiles"], config.Profile);
	        this.DefaultWorkDir = source["DefaultWorkDir"];
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
	export class CreateHostOptions {
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    authMethod: string;
	    keyPath: string;
	    secret: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateHostOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	        this.secret = source["secret"];
	    }
	}
	export class CreateOptions {
	    title: string;
	    path: string;
	    program: string;
	    branch: string;
	    autoYes: boolean;
	    inPlace: boolean;
	    prompt: string;
	    hostId: string;
	    mcpEnabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.path = source["path"];
	        this.program = source["program"];
	        this.branch = source["branch"];
	        this.autoYes = source["autoYes"];
	        this.inPlace = source["inPlace"];
	        this.prompt = source["prompt"];
	        this.hostId = source["hostId"];
	        this.mcpEnabled = source["mcpEnabled"];
	    }
	}
	export class Definition {
	    name: string;
	    path: string;
	    line: number;
	    kind: string;
	    language: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new Definition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.line = source["line"];
	        this.kind = source["kind"];
	        this.language = source["language"];
	        this.scope = source["scope"];
	    }
	}
	export class DiffFileResult {
	    path: string;
	    oldContent: string;
	    newContent: string;
	    status: string;
	    submodule: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.oldContent = source["oldContent"];
	        this.newContent = source["newContent"];
	        this.status = source["status"];
	        this.submodule = source["submodule"];
	    }
	}
	export class DiffStats {
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new DiffStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	export class DirInfo {
	    defaultBranch: string;
	    branches: string[];
	
	    static createFrom(source: any = {}) {
	        return new DirInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultBranch = source["defaultBranch"];
	        this.branches = source["branches"];
	    }
	}
	export class DirectoryEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new DirectoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	    }
	}
	export class HostInfo {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    authMethod: string;
	    keyPath: string;
	    lastPath: string;
	
	    static createFrom(source: any = {}) {
	        return new HostInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	        this.lastPath = source["lastPath"];
	    }
	}
	export class RemoteDirEntry {
	    name: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RemoteDirEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    title: string;
	    path: string;
	    branch: string;
	    program: string;
	    status: string;
	    hostId: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.path = source["path"];
	        this.branch = source["branch"];
	        this.program = source["program"];
	        this.status = source["status"];
	        this.hostId = source["hostId"];
	    }
	}
	export class SessionStatus {
	    id: string;
	    status: string;
	    branch: string;
	    diffStats: DiffStats;
	    hasPrompt: boolean;
	    sshConnected?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.branch = source["branch"];
	        this.diffStats = this.convertValues(source["diffStats"], DiffStats);
	        this.hasPrompt = source["hasPrompt"];
	        this.sshConnected = source["sshConnected"];
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
	export class TestHostResult {
	    connectionOK: boolean;
	    programOK: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TestHostResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionOK = source["connectionOK"];
	        this.programOK = source["programOK"];
	        this.message = source["message"];
	    }
	}

}

export namespace config {
	
	export class Profile {
	    name: string;
	    program: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.program = source["program"];
	    }
	}

}

