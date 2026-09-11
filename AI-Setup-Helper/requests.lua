local R = {}

local json = require("json")

local function direct_payload(data, api_key)
    local url = "https://openrouter.ai/api/v1/chat/completions"
    local headers = {
        ["Authorization"] = "Bearer " .. api_key,
        ["Content-Type"] = "application/json",
    }

    ac.debug("LLM Data", data)

    -- According to research I did, the ranking of the models
    -- based on performance/price ratio is:
    -- deepseek-v4-flash
    -- deepseek-v4-pro
    -- Gemini 2.5 Flash
    local prompt = (App_Settings.prompt or "") .. data

    local payload_table = {
        model = App_Settings.model or "google/gemini-3.7-flash",
        messages = {
            { role = "system", content = App_Settings.instructions or "" },
            { role = "user", content = prompt }
        },
        max_tokens = 2500,
        temperature = 0.2
    }

    -- Gemini requires reasoning, but others just start yapping and return garbage instead of json
    if (App_Settings.model or ""):find("gemini") then
        payload_table.reasoning = { enabled = true }
    end

    local payload = json.encode(payload_table)

    return url, headers, payload
end

local function common_payload(data, api_key)
    local url = "https://ai-setup-helper-ac.onrender.com/setup"

    if CachedMachineHash == nil then
        getHash()
    end

    local headers = {
        ["Content-Type"] = "application/json",
        ["X-Machine-Hash"] = CachedMachineHash
    }

    ac.debug("LLM Data", data)
    
    return url, headers, data
end

--- Makes the request to the chosen provider
--- @param common   boolean
---@param data      string
---@param api_key   string
---@param callback function
function R.make_request(common, data, api_key, callback)
    local url, headers, payload
    
    if common then
        url, headers, payload = common_payload(data, api_key)
    else
        url, headers, payload = direct_payload(data, api_key)
    end

    if type(payload) ~= "string" then
        callback("Failed to encode request payload.", false)
        return
    end
    
    -- Set and make request here
    web.request(
        "POST",
        url,
        headers,
        payload,
        function(err, response)
            local status = response and response.status or nil
            if err or not status or status < 200 or status >= 300 then
                if response then
                    if response.status == 429 then
                        callback("Rate limit exceeded. Try again later.", false)
                    elseif response.status == 401 then
                        callback("Unauthorized. Check your API key.", false)
                    elseif response.status == 502 then
                        callback("Chosen provider is down or not responding. Try again later.", false)
                    elseif response.status == 402 then
                        callback("One of us screwed up. And ran out of credits.\n(If you use the common provider, it was me, sorry)", false)
                    elseif response.status == 413 then
                        callback("Body size too large", false)
                    elseif response.status == 204 then
                        callback("The stupid AI responded, but with nothing :|", false)
                    elseif response.status == 500 then
                        callback("Something broke in the app. Try again in a bit", false)
                    elseif response.status == 400 then
                        callback("Bad request. Check your input.", false)
                    else
                        callback("Request failed. Try again in a bit", false)
                    end
                else
                    callback(err or "Could not make the request. Try again in a bit.", false)
                end

                return
            end
            
            local content = ""

            if common then
                content = response.body
            else
                local ok, body = pcall(json.decode, response.body)
                if not ok or not body then
                    callback("Invalid JSON response from provider.", false)
                    return
                end
                if not body
                    or not body.choices
                    or not body.choices[1]
                    or not body.choices[1].message then
                    callback("Invalid response from provider.", false)
                    return
                end

                ac.debug("LLM Response", body.choices[1].message.content)
                content = body.choices[1].message.content
            end

            -- Clean json
            content = content:gsub("^```json%s*", ""):gsub("^```%s*", ""):gsub("```%s*$", "")
            callback(content, true)
        end
    )

end

return R
