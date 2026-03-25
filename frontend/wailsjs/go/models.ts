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
	export class CreateOptions {
	    title: string;
	    path: string;
	    program: string;
	    branch: string;
	    autoYes: boolean;
	    inPlace: boolean;
	    prompt: string;
	
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
	export class SessionInfo {
	    id: string;
	    title: string;
	    path: string;
	    branch: string;
	    program: string;
	    status: string;
	
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
	    }
	}
	export class SessionStatus {
	    id: string;
	    status: string;
	    branch: string;
	    diffStats: DiffStats;
	    hasPrompt: boolean;
	
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

