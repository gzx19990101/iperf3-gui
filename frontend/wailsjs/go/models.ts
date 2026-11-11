export namespace model {
	
	export class AppError {
	    code: string;
	    stage: string;
	    message: string;
	    detail?: string;
	    runId?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.stage = source["stage"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.runId = source["runId"];
	    }
	}
	export class ClientConfig {
	    host: string;
	    port: number;
	    protocol: string;
	    direction: string;
	    durationSeconds: number;
	    intervalSeconds: number;
	    parallelStreams: number;
	    bindAddress?: string;
	    ipVersion?: number;
	    udpBandwidthBitsPerSecond?: number;
	    udpDatagramLengthBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new ClientConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.direction = source["direction"];
	        this.durationSeconds = source["durationSeconds"];
	        this.intervalSeconds = source["intervalSeconds"];
	        this.parallelStreams = source["parallelStreams"];
	        this.bindAddress = source["bindAddress"];
	        this.ipVersion = source["ipVersion"];
	        this.udpBandwidthBitsPerSecond = source["udpBandwidthBitsPerSecond"];
	        this.udpDatagramLengthBytes = source["udpDatagramLengthBytes"];
	    }
	}
	export class DeleteResult {
	    deleted: number;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new DeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deleted = source["deleted"];
	        this.error = this.convertValues(source["error"], AppError);
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
	export class DirectionSummary {
	    bitsPerSecond: number;
	    bytes: number;
	    seconds: number;
	    tcpRetransmits?: number;
	    udpJitterMilliseconds?: number;
	    udpLostDatagrams?: number;
	    udpTotalDatagrams?: number;
	    udpLostPercent?: number;
	
	    static createFrom(source: any = {}) {
	        return new DirectionSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bitsPerSecond = source["bitsPerSecond"];
	        this.bytes = source["bytes"];
	        this.seconds = source["seconds"];
	        this.tcpRetransmits = source["tcpRetransmits"];
	        this.udpJitterMilliseconds = source["udpJitterMilliseconds"];
	        this.udpLostDatagrams = source["udpLostDatagrams"];
	        this.udpTotalDatagrams = source["udpTotalDatagrams"];
	        this.udpLostPercent = source["udpLostPercent"];
	    }
	}
	export class EngineCapabilities {
	    jsonStream: boolean;
	    jsonStreamFullOutput: boolean;
	    bidirTcp: boolean;
	    bidirUdp: boolean;
	    udp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EngineCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jsonStream = source["jsonStream"];
	        this.jsonStreamFullOutput = source["jsonStreamFullOutput"];
	        this.bidirTcp = source["bidirTcp"];
	        this.bidirUdp = source["bidirUdp"];
	        this.udp = source["udp"];
	    }
	}
	export class NetworkInterface {
	    name: string;
	    address: string;
	    family: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.address = source["address"];
	        this.family = source["family"];
	    }
	}
	export class EngineStatus {
	    available: boolean;
	    path?: string;
	    version?: string;
	    capabilities: EngineCapabilities;
	    interfaces: NetworkInterface[];
	    error?: AppError;
	    probing?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EngineStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.capabilities = this.convertValues(source["capabilities"], EngineCapabilities);
	        this.interfaces = this.convertValues(source["interfaces"], NetworkInterface);
	        this.error = this.convertValues(source["error"], AppError);
	        this.probing = source["probing"];
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
	export class ExportResultInfo {
	    canceled: boolean;
	    path?: string;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new ExportResultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.canceled = source["canceled"];
	        this.path = source["path"];
	        this.error = this.convertValues(source["error"], AppError);
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
	export class HistoryFilter {
	    query: string;
	    protocol: string;
	    direction: string;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.protocol = source["protocol"];
	        this.direction = source["direction"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	    }
	}
	export class HistoryItem {
	    id: string;
	    createdAt: string;
	    startedAt?: string;
	    endedAt?: string;
	    status: string;
	    protocol: string;
	    direction: string;
	    host: string;
	    port: number;
	    durationSeconds: number;
	    uploadBitsPerSecond?: number;
	    downloadBitsPerSecond?: number;
	    exitCode?: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = source["createdAt"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.status = source["status"];
	        this.protocol = source["protocol"];
	        this.direction = source["direction"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.durationSeconds = source["durationSeconds"];
	        this.uploadBitsPerSecond = source["uploadBitsPerSecond"];
	        this.downloadBitsPerSecond = source["downloadBitsPerSecond"];
	        this.exitCode = source["exitCode"];
	    }
	}
	export class HistoryPage {
	    items: HistoryItem[];
	    total: number;
	    page: number;
	    pageSize: number;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], HistoryItem);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.error = this.convertValues(source["error"], AppError);
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
	export class Interval {
	    runId: string;
	    sequence: number;
	    direction: string;
	    startSeconds: number;
	    endSeconds: number;
	    bitsPerSecond: number;
	    bytes?: number;
	    tcpRetransmits?: number;
	    udpJitterMilliseconds?: number;
	    udpLostDatagrams?: number;
	    udpTotalDatagrams?: number;
	
	    static createFrom(source: any = {}) {
	        return new Interval(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.sequence = source["sequence"];
	        this.direction = source["direction"];
	        this.startSeconds = source["startSeconds"];
	        this.endSeconds = source["endSeconds"];
	        this.bitsPerSecond = source["bitsPerSecond"];
	        this.bytes = source["bytes"];
	        this.tcpRetransmits = source["tcpRetransmits"];
	        this.udpJitterMilliseconds = source["udpJitterMilliseconds"];
	        this.udpLostDatagrams = source["udpLostDatagrams"];
	        this.udpTotalDatagrams = source["udpTotalDatagrams"];
	    }
	}
	export class Summary {
	    direction: string;
	    upload?: DirectionSummary;
	    download?: DirectionSummary;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direction = source["direction"];
	        this.upload = this.convertValues(source["upload"], DirectionSummary);
	        this.download = this.convertValues(source["download"], DirectionSummary);
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
	export class RunRecord {
	    id: string;
	    createdAt: string;
	    startedAt?: string;
	    endedAt?: string;
	    status: string;
	    protocol: string;
	    direction: string;
	    host: string;
	    port: number;
	    config: ClientConfig;
	    outputMode?: string;
	    parserVersion?: string;
	    exitCode?: number;
	    summary?: Summary;
	
	    static createFrom(source: any = {}) {
	        return new RunRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = source["createdAt"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.status = source["status"];
	        this.protocol = source["protocol"];
	        this.direction = source["direction"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.config = this.convertValues(source["config"], ClientConfig);
	        this.outputMode = source["outputMode"];
	        this.parserVersion = source["parserVersion"];
	        this.exitCode = source["exitCode"];
	        this.summary = this.convertValues(source["summary"], Summary);
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
	export class HistoryResult {
	    run: RunRecord;
	    intervals: Interval[];
	    intervalTotal: number;
	    page: number;
	    pageSize: number;
	    rawOutput?: string;
	    rawTruncated?: boolean;
	    stderrSummary?: string;
	    stderrTruncated?: boolean;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new HistoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], RunRecord);
	        this.intervals = this.convertValues(source["intervals"], Interval);
	        this.intervalTotal = source["intervalTotal"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.rawOutput = source["rawOutput"];
	        this.rawTruncated = source["rawTruncated"];
	        this.stderrSummary = source["stderrSummary"];
	        this.stderrTruncated = source["stderrTruncated"];
	        this.error = this.convertValues(source["error"], AppError);
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
	
	
	
	export class RunSnapshot {
	    runId: string;
	    status: string;
	    config: ClientConfig;
	    startedAt?: string;
	    endedAt?: string;
	    lastSequence: number;
	    latestIntervals?: Interval[];
	    summary?: Summary;
	    exitCode?: number;
	    error?: AppError;
	    message?: string;
	    rawOutput?: string;
	    rawTruncated?: boolean;
	    stderrSummary?: string;
	    stderrTruncated?: boolean;
	    historyId?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.status = source["status"];
	        this.config = this.convertValues(source["config"], ClientConfig);
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	        this.lastSequence = source["lastSequence"];
	        this.latestIntervals = this.convertValues(source["latestIntervals"], Interval);
	        this.summary = this.convertValues(source["summary"], Summary);
	        this.exitCode = source["exitCode"];
	        this.error = this.convertValues(source["error"], AppError);
	        this.message = source["message"];
	        this.rawOutput = source["rawOutput"];
	        this.rawTruncated = source["rawTruncated"];
	        this.stderrSummary = source["stderrSummary"];
	        this.stderrTruncated = source["stderrTruncated"];
	        this.historyId = source["historyId"];
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
	export class ServerConfig {
	    port: number;
	    bindAddress?: string;
	    oneOff: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.bindAddress = source["bindAddress"];
	        this.oneOff = source["oneOff"];
	    }
	}
	export class StartRunResult {
	    runId?: string;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new StartRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.error = this.convertValues(source["error"], AppError);
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
	export class StartServerResult {
	    serverId?: string;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new StartServerResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.error = this.convertValues(source["error"], AppError);
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
	export class StopResult {
	    status: string;
	    error?: AppError;
	
	    static createFrom(source: any = {}) {
	        return new StopResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.error = this.convertValues(source["error"], AppError);
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

