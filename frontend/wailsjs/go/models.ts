export namespace engine {
	
	export class ResourceEntry {
	    kind: string;
	    target: string;
	    ports: string;
	    protocol: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.target = source["target"];
	        this.ports = source["ports"];
	        this.protocol = source["protocol"];
	    }
	}

}

export namespace main {
	
	export class Profile {
	    id: string;
	    protocol: string;
	    domain: string;
	    insecureSkipVerify: boolean;
	    socksPort: number;
	    httpPort: number;
	    name: string;
	    server: string;
	    port: number;
	    username: string;
	    password: string;
	    enable2FA: boolean;
	    totpSecret: string;
	    bindInterface: string;
	    tunMode: boolean;
	    disableProxy: boolean;
	    disableZjuConfig: boolean;
	    disableKeepAlive: boolean;
	    keepAliveUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.protocol = source["protocol"];
	        this.domain = source["domain"];
	        this.insecureSkipVerify = source["insecureSkipVerify"];
	        this.socksPort = source["socksPort"];
	        this.httpPort = source["httpPort"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.enable2FA = source["enable2FA"];
	        this.totpSecret = source["totpSecret"];
	        this.bindInterface = source["bindInterface"];
	        this.tunMode = source["tunMode"];
	        this.disableProxy = source["disableProxy"];
	        this.disableZjuConfig = source["disableZjuConfig"];
	        this.disableKeepAlive = source["disableKeepAlive"];
	        this.keepAliveUrl = source["keepAliveUrl"];
	    }
	}
	export class AppConfig {
	    activeProfileId: string;
	    profiles: Profile[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.activeProfileId = source["activeProfileId"];
	        this.profiles = this.convertValues(source["profiles"], Profile);
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
	export class PlatformInfo {
	    tunSupported: boolean;
	    logDir: string;
	
	    static createFrom(source: any = {}) {
	        return new PlatformInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tunSupported = source["tunSupported"];
	        this.logDir = source["logDir"];
	    }
	}
	
	export class VPNStatus {
	    profileId: string;
	    protocol: string;
	    running: boolean;
	    stage: string;
	    stageDetail: string;
	    stageStartedAt: number;
	    message: string;
	    state: string;
	    activeProfile: string;
	    clientIp: string;
	    dns: string;
	    socksPort: string;
	    httpPort: string;
	    tunMode: boolean;
	    tunState: string;
	    tunError: string;
	    disableProxy: boolean;
	    duration: number;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new VPNStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.protocol = source["protocol"];
	        this.running = source["running"];
	        this.stage = source["stage"];
	        this.stageDetail = source["stageDetail"];
	        this.stageStartedAt = source["stageStartedAt"];
	        this.message = source["message"];
	        this.state = source["state"];
	        this.activeProfile = source["activeProfile"];
	        this.clientIp = source["clientIp"];
	        this.dns = source["dns"];
	        this.socksPort = source["socksPort"];
	        this.httpPort = source["httpPort"];
	        this.tunMode = source["tunMode"];
	        this.tunState = source["tunState"];
	        this.tunError = source["tunError"];
	        this.disableProxy = source["disableProxy"];
	        this.duration = source["duration"];
	        this.lastError = source["lastError"];
	    }
	}

}

