local R = {}

local requests = require "http.request"

--- Makes a post request to the LLM provider
---@param data    string
---@param api_key string
---@return string|nil
---@return string|nil
local function make_local_request(data, api_key)
    local url = "https://openrouter.ai/api/v1/chat/completions"
    local headers = {
        ["Authorization"] = "Bearer " .. api_key,
    }

    local model = ""
    local prompt = ""

    local req = requests.new_from_uri(url)
    req.headers:upsert(":method", "POST")
    req.headers:upsert("content-type", "application/json")

    req.set_body('{"model":'..model..',"messages": [{"role": "user", "content": "'..prompt..'"}]}')

    -- Make request
    local headers, stream = assert(req:go())

    local response_status = headers.get(":status")
    response_status = tonumber(response_status)
    if response_status ~= 200 then
        return nil, nil
    elseif response_status == 429 then
        return nil, "Request Limit Reached"
    end

    -- Get response body
    local response = assert(stream:get_body_as_string())

    return response, nil
end

-- TODO: backend request

R.make_local_request = make_local_request