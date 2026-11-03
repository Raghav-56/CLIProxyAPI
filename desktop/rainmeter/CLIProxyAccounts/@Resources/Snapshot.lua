local path
local expanded = false
local detailRows = {SelectedDetail=219, LatestDetail=242, ModelDetail=264, TimingDetail=286, TokensDetail=308, PoolDetail=331}
local colors = {text='229,231,223', muted='165,176,166', gold='216,182,116', bad='218,137,124', green='157,191,161'}

local function safe(s)
    return type(s) == 'string' and #s <= 180 and not s:find('[^a-zA-Z0-9 .,:_()/+%-]')
end
local function number(s, low, high)
    local n = tonumber(s)
    if n and n == n and n >= low and n <= high then return n end
end
local function snapshotLive(written, now)
    return written and now >= written and now-written <= 35
end
local function quotaLive(untilTime, now)
    return untilTime and untilTime > now
end
local function duration(minutes)
    local n = number(minutes, 1, 5256000)
    if not n or n % 1 ~= 0 then return 'Quota window' end
    if n == 10080 then return 'Weekly' end
    if n % 1440 == 0 then return string.format('%.0f-day', n/1440) end
    if n % 60 == 0 then return string.format('%.0f-hour', n/60) end
    return string.format('%.0f-minute', n)
end
local function relative(seconds)
    local n = math.max(0, math.floor(seconds))
    if n < 60 then return '<1m' end
    if n < 3600 then return math.floor(n/60)..'m' end
    if n < 86400 then
        local m = math.floor(n%3600/60)
        return math.floor(n/3600)..'h'..(m > 0 and ' '..m..'m' or '')
    end
    local h = math.floor(n%86400/3600)
    return math.floor(n/86400)..'d'..(h > 0 and ' '..h..'h' or '')
end

function BuildView(values, now)
    local written = number(values.Written, 0, 253402300799)
    local live = snapshotLive(written, now)
    local connected = live and values.Connection == 'Connected'
    local mode = values.RoutingMode
    local plan = values.SelectedPlan or 'unknown'
    local context = 'Open Accounts to connect'
    if connected then
        if mode == 'manual' then
            context = (plan == 'unknown' and 'Plan unknown' or plan:gsub('^%l', string.upper))..' / Manual'
        elseif mode == 'soonest-reset' then context = 'Soonest reset / Session affinity'
        elseif mode == 'home' then context = 'Managed by Home'
        else context = 'Automatic routing' end
        if values.RoutingApplied ~= '1' then context = context..' / Not applied' end
    elseif live then context = values.Connection or 'Connection unavailable'
    else context = 'Helper stopped / Open Accounts' end
    local view = {Context=context, StatusColor=connected and colors.green or colors.bad}
    local valid = connected and mode == 'manual' and quotaLive(number(values.SelectedUntil, 0, 253402300799), now)
        and (values.SelectedQuotaState == 'available' or values.SelectedQuotaState == 'exhausted')
    if connected and mode == 'manual' then
        if values.SelectedState == 'unavailable' then
            view.Context = context..' / Unavailable'
            view.StatusColor = colors.bad
        elseif not valid then
            view.Context = context..' / Quota unknown'
            view.StatusColor = colors.muted
        elseif values.SelectedState ~= 'available' and values.SelectedState ~= 'exhausted' then
            view.Context = context..' / Status unknown'
            view.StatusColor = colors.muted
        elseif values.SelectedQuotaState == 'exhausted' then
            view.Context = context..' / Limit reached'
            view.StatusColor = colors.bad
        end
    end
    for _, name in ipairs({'Primary','Secondary'}) do
        local remaining = valid and number(values[name..'Remaining'], 0, 100)
        local reset = valid and number(values[name..'Reset'], 0, 253402300799)
        if not reset or reset <= now then remaining = nil end
        view[name] = {
            Label=duration(values[name..'Duration']),
            Value=remaining and string.format('%.0f%%', remaining) or '--',
            Reset=remaining and ('Resets '..relative(reset-now)) or (mode == 'manual' and 'Refresh in Accounts' or 'No manual selection'),
            Width=remaining and remaining*124/100 or 0,
            Color=remaining and (remaining <= 15 and colors.bad or colors.gold) or colors.muted,
        }
    end
    local a = number(values.PoolAvailable, 0, 1000000)
    local b = number(values.PoolBlocked, 0, 1000000)
    local u = number(values.PoolUnknown, 0, 1000000)
    if connected and a and b and u and quotaLive(number(values.PoolUntil, 0, 253402300799), now) then
        view.Pool = string.format('Pool: %.0f ready', a)
        if b > 0 then view.Pool = view.Pool..string.format(', %.0f blocked', b) end
        if u > 0 then view.Pool = view.Pool..string.format(', %.0f unknown', u) end
    else view.Pool = 'Pool needs a fresh observation' end
    local connectedAt = number(values.ConnectedAt, 0, now)
    view.Freshness = connectedAt and ('Updated '..relative(now-connectedAt)..' ago') or 'Not connected'
    if not connected then view.Freshness = 'Offline' end
    if connected then
        view.SelectedDetail = 'No manual selection'
        if mode == 'manual' then
            local state = values.SelectedState or 'unknown'
            if not valid and (state == 'available' or state == 'exhausted') then state = 'quota unknown' end
            view.SelectedDetail = (values.SelectedID or 'unknown')..' / '..plan..' / '..state
        end
        local id, outcome = (values.Attempt or ''):match('Latest (acct%-%x+) / (%a+)')
        local age = number((values.AttemptAge or ''):match('^(%d+)s$'), 0, 253402300799)
        view.LatestDetail = id and ('Last '..id..' / '..outcome..(age and ' / '..relative(age+now-written)..' ago' or '')) or 'No recent attempts'
        view.ModelDetail = values.Model or 'Model unknown'
        local transportNames = {http='HTTP', sse='SSE', websocket='WS'}
        local down, up = (values.Transport or ''):match('^(%a+) to (%a+)$')
        local elapsed = number((values.Timing or ''):match('Elapsed (%d+)ms'),0,1e15)
        local ttft = number((values.Timing or ''):match('TTFT (%d+)ms'),0,1e15)
        view.TimingDetail = (transportNames[down] or '?')..' > '..(transportNames[up] or '?')..' / '..
            (elapsed and string.format('%.2fs',elapsed/1000) or 'unknown')..' / TTFT '..
            (ttft and string.format('%.2fs',ttft/1000) or 'unknown')
        view.TokensDetail = values.Tokens or 'Tokens unknown'
        view.PoolDetail = quotaLive(number(values.PoolUntil, 0, 253402300799), now) and
            (values.PoolList or 'Pool details unknown') or 'Pool observations expired / awaiting update'
    else
        for name in pairs(detailRows) do view[name] = 'Unavailable while disconnected' end
    end
    return view
end

local function text(name, value)
    SKIN:Bang('!SetOption', name, 'Text', value)
    SKIN:Bang('!SetOption', name, 'ToolTipText', value)
end
local function layout()
    local height = expanded and 420 or 246
    SKIN:Bang('!SetOption','Panel','Shape','Rectangle 0,0,304,'..height..',12 | Fill Color 23,28,26,250 | Stroke Color 57,66,60')
    for name, y in pairs(detailRows) do
        SKIN:Bang('!SetOption', name, 'Y', expanded and y or 0)
        SKIN:Bang('!SetOption', name, 'H', expanded and (name == 'PoolDetail' and 42 or 18) or 0)
    end
    SKIN:Bang(expanded and '!ShowMeterGroup' or '!HideMeterGroup', 'Details')
    SKIN:Bang('!SetOption','DetailsButton','Text',expanded and 'Less' or 'Details')
    SKIN:Bang('!SetOption','Console','Y',height-29)
    SKIN:Bang('!SetOption','Freshness','Y',height-29)
    SKIN:Bang('!SetOption','FooterLine','Y',height-40)
    SKIN:Bang('!UpdateMeter','*')
    SKIN:Bang('!Redraw')
end
function Initialize()
    path = os.getenv('LOCALAPPDATA')..'\\CLIProxyAccounts\\snapshot.ini'
    layout()
end
function ToggleDetails()
    expanded = not expanded
    layout()
end
function Update()
    local values = {}
    local file = io.open(path,'r')
    if file then
        for line in file:lines() do
            local key, value = line:match('^(%a+)=(.*)$')
            if key and safe(value) then values[key] = value end
        end
        file:close()
    end
    local view = BuildView(values,os.time())
    for _,name in ipairs({'Context','Pool','Freshness','SelectedDetail','LatestDetail','ModelDetail','TimingDetail','TokensDetail','PoolDetail'}) do text(name,view[name]) end
    SKIN:Bang('!SetOption','Pool','ToolTipText','Ready means fresh usable quota. Model eligibility and session affinity still affect routing.')
    SKIN:Bang('!SetOption','Status','Shape','Ellipse 3,3,3 | Fill Color '..view.StatusColor..' | StrokeWidth 0')
    for _,name in ipairs({'Primary','Secondary'}) do
        local q = view[name]
        text(name..'Label',q.Label)
        text(name..'Value',q.Value)
        text(name..'Reset',q.Reset)
        SKIN:Bang('!SetOption',name..'Value','FontColor',q.Color)
        SKIN:Bang('!SetOption',name..'Fill','Shape',string.format('Rectangle 0,0,%.2f,4,2 | Fill Color %s | StrokeWidth 0',q.Width,q.Color))
    end
    SKIN:Bang('!UpdateMeter','*')
    return 0
end
function SelfTest()
    for _,s in ipairs({'[!Quit]','#Key#','[Measure]','%ENV%','line\nbreak','line\rbreak','a=b','"quoted"',string.rep('a',181)}) do assert(not safe(s)) end
    assert(safe('acct-12345678 / available'))
    assert(snapshotLive(100,135) and not snapshotLive(100,136) and not snapshotLive(100,99))
    assert(quotaLive(300,299) and not quotaLive(300,300))
    assert(duration('300') == '5-hour' and duration('10080') == 'Weekly' and duration('43200') == '30-day')
    assert(relative(152*60) == '2h 32m' and relative(9629*60) == '6d 16h')
    local fixture = {Written='100', ConnectedAt='100', Connection='Connected', RoutingMode='manual', RoutingApplied='1', SelectedPlan='plus', SelectedID='acct-12345678', SelectedState='available', SelectedQuotaState='available', SelectedUntil='300', PrimaryDuration='300', PrimaryRemaining='44', PrimaryReset='1000', SecondaryDuration='10080', SecondaryRemaining='76', SecondaryReset='2000', PoolAvailable='1',PoolBlocked='0',PoolUnknown='2',PoolUntil='300',PoolList='acct-12345678 available'}
    local view = BuildView(fixture,110)
    assert(view.Context == 'Plus / Manual' and view.Primary.Value == '44%' and view.Secondary.Value == '76%')
    assert(view.Pool == 'Pool: 1 ready, 2 unknown' and math.abs(view.Primary.Width-54.56) < 0.001)
    assert(BuildView(fixture,136).Primary.Value == '--')
    fixture.SelectedState='unavailable'
    assert(BuildView(fixture,110).Context == 'Plus / Manual / Unavailable' and BuildView(fixture,110).StatusColor == colors.bad)
    fixture.SelectedState='unknown'; assert(BuildView(fixture,110).Context == 'Plus / Manual / Status unknown')
    fixture.SelectedState='available'
    fixture.SelectedUntil='110'; assert(BuildView(fixture,110).Primary.Value == '--' and not BuildView(fixture,110).SelectedDetail:find('available'))
    fixture.PoolUntil='110'; assert(not BuildView(fixture,110).PoolDetail:find('available'))
    fixture.SelectedUntil='300'; fixture.PrimaryRemaining='0'; assert(BuildView(fixture,110).Primary.Value == '0%')
    fixture.RoutingMode='soonest-reset'; assert(BuildView(fixture,110).Primary.Value == '--')
    print('CLIProxyAccounts Lua SelfTest PASS: compact view, percentages, reset countdowns, automatic mode and freshness')
end
function LayoutTest()
    local height = expanded and 420 or 246
    for _,name in ipairs({'Title','Context','PrimaryLabel','PrimaryValue','PrimaryReset','SecondaryLabel','SecondaryValue','SecondaryReset','Pool','Console','Freshness'}) do
        local meter = SKIN:GetMeter(name)
        assert(meter and meter:GetY() >= 0 and meter:GetY()+meter:GetH() <= height, name..' outside panel')
    end
    if expanded then
        for name in pairs(detailRows) do
            local meter=SKIN:GetMeter(name)
            assert(meter:GetY()+meter:GetH() <= height, name..' outside expanded panel')
        end
    end
    print('CLIProxyAccounts LayoutTest PASS: '..(expanded and 'details' or 'compact')..' meter bounds')
end
