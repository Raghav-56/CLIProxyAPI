local fields = {'Connection','Route','Selected','Primary','Secondary','Observation','Pool','PoolList','Attempt','Model','Transport','Timing','Tokens','AttemptAge'}
local path
function Initialize()
    path = os.getenv('LOCALAPPDATA') .. '\\CLIProxyAccounts\\snapshot.ini'
end
local function safe(s)
    return type(s) == 'string' and #s <= 180 and not s:find('[^a-zA-Z0-9 .,:_()/+%-]')
end
local function snapshotLive(written, now)
    return written and now >= written and now-written <= 35
end
local function quotaLive(untilTime, now)
    return untilTime and untilTime > now
end
function SelfTest()
    assert(safe('acct-12345678 / available'))
    for _,s in ipairs({'[!Quit]','#Key#','[Measure]','%ENV%','line\nbreak','line\rbreak','a=b','"quoted"',string.rep('a',181)}) do
        assert(not safe(s))
    end
    assert(snapshotLive(100,135) and not snapshotLive(100,136))
    assert(not snapshotLive(100,99) and not snapshotLive(nil,100))
    assert(quotaLive(300,299) and not quotaLive(300,300) and not quotaLive(nil,300))
    print('CLIProxyAccounts Lua SelfTest PASS: sanitization, stopped helper, freshness and reset boundaries')
end
function Update()
    local values = {}
    local f = io.open(path, 'r')
    if f then
        for line in f:lines() do
            local k,v = line:match('^(%a+)=(.*)$')
            if k and safe(v) then values[k] = v end
        end
        f:close()
    end
    local now = os.time()
    local written = tonumber(values.Written)
    local age = written and now-written or 999999
    local live = snapshotLive(written, now)
    local quotaUntil = tonumber(values.QuotaUntil)
    for _,name in ipairs(fields) do
        local value = values[name] or 'unknown'
        if not live then value = name == 'Connection' and 'Helper stopped or snapshot old' or 'unknown' end
        if live and not quotaLive(quotaUntil, now) and
            (name == 'Primary' or name == 'Secondary' or name == 'Pool' or name == 'PoolList' or name == 'Selected') then
            value = 'Quota observation expired / awaiting poll'
        end
        if name == 'Observation' and live then
            local seconds = tonumber(value:match('^(%d+)s$'))
            value = seconds and ('Quota observation '..(seconds+age)..'s ago') or 'Quota observation unknown'
        end
        if name == 'AttemptAge' and live then
            local seconds = tonumber(value:match('^(%d+)s$'))
            value = seconds and ('Attempt '..(seconds+age)..'s ago') or 'Attempt age unknown'
        end
        SKIN:Bang('!SetOption',name,'Text',value)
    end
    local connected = tonumber(values.ConnectedAt)
    local text = connected and connected <= now and ('Last connected '..(now-connected)..'s ago') or 'Never connected'
    SKIN:Bang('!SetOption','ConnectedAge','Text',text)
    SKIN:Bang('!UpdateMeter','*')
    return 0
end
