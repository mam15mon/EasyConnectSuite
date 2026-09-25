export namespace main {
	
	export class Profile {
	    id: string;
	    name: string;
	    server: string;
	    port: number;
	    username: string;
	    password: string;
	    enable2FA: boolean;
	    totpSecret: string;
	    bindInterface: string;
	    disableZjuConfig: boolean;
	    disableKeepAlive: boolean;
	    keepAliveUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.enable2FA = source["enable2FA"];
	        this.totpSecret = source["totpSecret"];
	        this.bindInterface = source["bindInterface"];
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
	
	export class VPNStatus {
	    running: boolean;
	    state: string;
	    activeProfile: string;
	    clientIp: string;
	    dns: string;
	    socksPort: string;
	    httpPort: string;
	    duration: number;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new VPNStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.state = source["state"];
	        this.activeProfile = source["activeProfile"];
	        this.clientIp = source["clientIp"];
	        this.dns = source["dns"];
	        this.socksPort = source["socksPort"];
	        this.httpPort = source["httpPort"];
	        this.duration = source["duration"];
	        this.lastError = source["lastError"];
	    }
	}

}

