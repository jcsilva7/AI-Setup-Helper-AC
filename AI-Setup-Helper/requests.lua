local R = {}

--- Makes a post request to the LLM provider
---@param data    string
---@param api_key string
---@param callback function
local function make_local_request(data, api_key, callback)
    local url = "https://openrouter.ai/api/v1/chat/completions"
    local headers = {
        ["Authorization"] = "Bearer " .. api_key,
        ["Content-Type"] = "application/json",
    }

    local model = ""
    local prompt = ""

    local response, status
    web.request(
        "POST",
        url,
        headers,
        '{"model":'..model..',"messages": [{"role": "user", "content": "'..prompt..'"}]}',
        function(err, response)
            if err then
                if response and response.status == 429 then
                    callback("Rate limit exceeded. Try again later.", false)
                else
                    callback(err, false)
                end

                return
            end
            
            callback(response.body, true)
        end
    )

end

-- TODO: backend request

R.make_local_request = make_local_request