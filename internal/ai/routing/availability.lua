local action, token = ARGV[1], ARGV[2]
local lease = tonumber(ARGV[3])
local scopes, verdict = cjson.decode(ARGV[4]), cjson.decode(ARGV[5])
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local prefix = 'dai:availability:v2:'
local permitKey = prefix..'permit:'..token
local ttl = 7*24*60*60*1000

local function load(key, scope)
  local raw = redis.call('GET', key)
  local s
  if raw then s = cjson.decode(raw)
  else s = {scope=scope, phase='available', epoch=0, trips=0, consecutive_failures=0,
    recovery_successes=0, recent_trips={}, window={}, stable_successes=0} end
  if s.phase == 'cooling' and (s.retry_at or 0) <= now then s.phase='recovering' end
  if (s.lease_until or 0) <= now then s.owner=nil; s.lease_until=0 end
  local trips = {}
  for _, t in ipairs(s.recent_trips or {}) do if t > now-3600000 then table.insert(trips,t) end end
  s.recent_trips=trips
  return s
end
local function save(key, s)
  redis.call('SET',key,cjson.encode(s),'PX',math.max(ttl,(s.retry_at or 0)-now+86400000))
  redis.call('SADD',prefix..'scopes',key)
  redis.call('SADD',prefix..'resource:'..s.scope.resource_kind..':'..s.scope.resource_id,key)
end
local function snapshot(s)
  local r = {busy=s.owner~=nil}
  for k,v in pairs(s) do if k ~= 'owner' and k ~= 'window' then r[k]=v end end
  -- Redis cjson represents an empty Lua table as an object. Keep public arrays stable.
  if #s.recent_trips == 0 then r.recent_trips=cjson.empty_array or nil end
  return r
end
local function array(items)
  if #items == 0 then return '[]' end
  return cjson.encode(items)
end
local function observe(s, failed)
  local sec = math.floor(now/1000)
  local total, failures, kept = 0,0,{}
  for k,b in pairs(s.window or {}) do
    if tonumber(k)>sec-60 then kept[k]=b; total=total+b[1]; failures=failures+b[2] end
  end
  local b = kept[tostring(sec)] or {0,0}
  b[1]=b[1]+1; if failed then b[2]=b[2]+1 end
  kept[tostring(sec)]=b; s.window=kept
  return total+1, failures+(failed and 1 or 0)
end
local function cool(s)
  s.trips=s.trips+1; s.epoch=s.epoch+1
  local delay=math.min(300000,30000*2^math.min(s.trips-1,4))
  if verdict.authentication then delay=1800000 end
  s.retry_at=now+delay
  if (verdict.cooldown_until or 0)>now and not verdict.authentication then s.retry_at=verdict.cooldown_until end
  s.phase='cooling'; s.owner=nil; s.lease_until=0; s.recovery_successes=0
  s.stable_at=nil; s.stable_successes=0
  table.insert(s.recent_trips,now)
end

if action=='cancel' then
 redis.call('SET',permitKey..':canceled','1','PX',math.max(lease,60000));action='complete';verdict={}
end
if action == 'complete' then
  local raw = redis.call('GET',permitKey)
  if not raw then return '[]' end
  local p = cjson.decode(raw)
  redis.call('DEL',permitKey)
  if p.expires_at<=now then return '[]' end
  local out={}
  for _, scope in ipairs(p.scopes) do
    local key=prefix..'scope:'..scope.key
    local s=load(key,scope)
    if s.epoch==p.epochs[scope.key] then
      -- A recovering lease reclaimed by another request fences this completion.
      local owns = s.phase~='recovering' or s.owner==token
      if owns then
        if s.owner==token then s.owner=nil; s.lease_until=0 end
        if verdict.success then
          s.verified_at=now; s.consecutive_failures=0
          if s.phase=='recovering' then
            s.recovery_successes=s.recovery_successes+1
            if s.recovery_successes>=2 then
              s.phase='available'; s.retry_at=0; s.window={}; s.stable_at=now; s.stable_successes=0
            end
          else
            observe(s,false)
            s.stable_at=s.stable_at or now; s.stable_successes=(s.stable_successes or 0)+1
            if now-s.stable_at>=600000 and s.stable_successes>=10 then s.trips=0 end
          end
        elseif verdict.failure_scope==scope.key then
          s.last_failure_at=now; s.reason=verdict.reason; s.stable_at=nil; s.stable_successes=0
          if not verdict.health_failure then cool(s)
          else
            s.consecutive_failures=s.consecutive_failures+1
            local n,f=observe(s,true)
            if s.phase=='recovering' or s.consecutive_failures>=5 or (n>=20 and f/n>=0.5) then cool(s) end
          end
        end
        save(key,s); table.insert(out,snapshot(s))
      end
    end
  end
  return array(out)
end

local states={}
for i,key in ipairs(KEYS) do states[i]=load(key,scopes[i]) end
if action=='import' or action=='suspend' then
 for i,s in ipairs(states) do
  if action=='suspend' or redis.call('EXISTS',KEYS[i])==0 then
   s.phase='cooling';s.epoch=s.epoch+1;s.retry_at=verdict.cooldown_until;s.reason=verdict.reason
   s.owner=nil;s.lease_until=0;s.recovery_successes=0;s.last_failure_at=now
   save(KEYS[i],s)
  end
 end
 return '[]'
end
if action=='read' then
  local out={}; for _,s in ipairs(states) do table.insert(out,snapshot(s)) end
  return array(out)
end
if action=='resume' or action=='reset' then
  for i,s in ipairs(states) do
   if action=='reset' or s.phase=='cooling' then
    s.epoch=s.epoch+1; s.phase='recovering'; s.retry_at=0; s.owner=nil; s.lease_until=0
    s.recovery_successes=0; s.window={}; s.consecutive_failures=0; s.stable_at=nil; s.stable_successes=0
    if action=='reset' then s.verified_at=nil end
    save(KEYS[i],s)
   end
  end
  return '[]'
end
if action=='acquire' then
 if redis.call('EXISTS',permitKey..':canceled')==1 then return cjson.encode({retry_at=0,reason='canceled'}) end
  local deniedAt,reason=0,''
  for _,s in ipairs(states) do
    if s.phase=='cooling' and s.retry_at>deniedAt then deniedAt=s.retry_at;reason=s.reason or 'cooling' end
    if s.phase=='recovering' and s.owner and s.lease_until>deniedAt then deniedAt=s.lease_until;reason='recovery_busy' end
  end
  if deniedAt>0 then return cjson.encode({retry_at=deniedAt,reason=reason}) end
  local p={token=token,epochs={},expires_at=now+lease,scopes=scopes}
  for i,s in ipairs(states) do
    p.epochs[s.scope.key]=s.epoch
    if s.phase=='recovering' then s.owner=token; s.lease_until=p.expires_at end
    save(KEYS[i],s)
  end
  redis.call('SET',permitKey,cjson.encode(p),'PX',lease)
  return cjson.encode({permit=p})
end
return redis.error_reply('unknown availability operation')
