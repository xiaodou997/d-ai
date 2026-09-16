-- One-time data cleanup before starting the fixed server. Run for each
-- dai:availability:v2:scope:* key in the application's Redis database.
-- Preserve cooldowns, epochs, leases, nonempty history and the original TTL.
if string.sub(KEYS[1], 1, 26) ~= 'dai:availability:v2:scope:' then return redis.error_reply('unexpected state key') end
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local state = cjson.decode(raw)
if type(state.recent_trips) ~= 'table' or next(state.recent_trips) ~= nil then return 0 end
state.recent_trips = nil
redis.call('SET', KEYS[1], cjson.encode(state), 'KEEPTTL')
return 1
