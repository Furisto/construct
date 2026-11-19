use super::*;
use crate::auth::{
    duplicate, expiry_duration as duration, issue_api_token, Identity, DEFAULT_SETUP_EXPIRY,
    DEFAULT_TOKEN_EXPIRY, MAX_SETUP_EXPIRY, MAX_TOKEN_EXPIRY,
};
use construct_types::memory::TokenType;

fn admin<T>(req: &Request<T>) -> Result<(), ConnectError> {
    if req
        .extensions
        .get::<Identity>()
        .is_some_and(|id| id.is_admin)
    {
        Ok(())
    } else {
        Err(ConnectError::new(
            Code::PermissionDenied,
            "admin privileges required",
        ))
    }
}
#[async_trait]
impl pb::AuthService for Services {
    async fn create_token(
        &self,
        req: Request<pb::CreateTokenRequest>,
    ) -> Result<Response<pb::CreateTokenResponse>, ConnectError> {
        admin(&req)?;
        Ok(Response::new(
            issue_api_token(self.store.as_ref(), &self.tokens, req.message).await?,
        ))
    }
    async fn create_setup_code(
        &self,
        req: Request<pb::CreateSetupCodeRequest>,
    ) -> Result<Response<pb::CreateSetupCodeResponse>, ConnectError> {
        admin(&req)?;
        let r = req.message;
        if r.token_name.is_empty() {
            return Err(invalid("token_name is required"));
        }
        let code_expiry = duration(
            r.expires_in,
            DEFAULT_SETUP_EXPIRY,
            MAX_SETUP_EXPIRY,
            "expires_in",
            "72h0m0s",
        )?;
        let token_expiry = duration(
            r.token_expires_in,
            DEFAULT_TOKEN_EXPIRY,
            MAX_TOKEN_EXPIRY,
            "token_expires_in",
            "8760h0m0s",
        )?;
        let code = self
            .tokens
            .create_setup_code(&r.token_name, code_expiry, token_expiry)
            .map_err(internal)?;
        Ok(Response::new(pb::CreateSetupCodeResponse {
            setup_code: code.code,
            expires_at: Some(conv::convert_time_to_timestamp(&code.expires_at)),
        }))
    }
    async fn list_tokens(
        &self,
        req: Request<pb::ListTokensRequest>,
    ) -> Result<Response<pb::ListTokensResponse>, ConnectError> {
        admin(&req)?;
        let r = req.message;
        let time = now();
        let tokens = self
            .store
            .list_tokens(
                TokenFilter {
                    name_prefix: r.name_prefix,
                    token_type: Some(TokenType::from(TokenType::API_TOKEN)),
                    expires_after: (!r.include_expired).then_some(time.clone()),
                    ..Default::default()
                },
                ListOptions::default(),
            )
            .await
            .api("token")?;
        Ok(Response::new(pb::ListTokensResponse {
            tokens: tokens
                .into_iter()
                .map(|t| pb::TokenInfo {
                    id: t.id.to_string(),
                    name: t.name,
                    description: (!t.description.is_empty()).then_some(t.description),
                    created_at: Some(conv::convert_time_to_timestamp(&t.create_time)),
                    expires_at: Some(conv::convert_time_to_timestamp(&t.expires_at)),
                    is_active: t.expires_at.0 > time.0,
                })
                .collect(),
        }))
    }
    async fn revoke_token(
        &self,
        req: Request<pb::RevokeTokenRequest>,
    ) -> Result<Response<pb::RevokeTokenResponse>, ConnectError> {
        admin(&req)?;
        let id = parse_id(&req.message.id, "ID")?;
        self.store.delete_token(id).await.api("token")?;
        Ok(Response::new(pb::RevokeTokenResponse {}))
    }
    async fn exchange_setup_code(
        &self,
        req: Request<pb::ExchangeSetupCodeRequest>,
    ) -> Result<Response<pb::ExchangeSetupCodeResponse>, ConnectError> {
        if req.message.setup_code.is_empty() {
            return Err(invalid("setup_code is required"));
        }
        let code = self
            .tokens
            .consume_setup_code(&req.message.setup_code)
            .ok_or_else(|| {
                ConnectError::new(Code::PermissionDenied, "invalid or expired setup code")
            })?;
        let tokens = self
            .store
            .list_tokens(TokenFilter::default(), ListOptions::default())
            .await
            .api("token")?;
        if tokens.iter().any(|t| t.name == code.token_name) {
            return Err(duplicate(&code.token_name));
        }
        let (token, hash) = self.tokens.generate_token().map_err(internal)?;
        let time = now();
        let expires = Timestamp(time.0 + code.token_expiry);
        self.store
            .create_token(db::Token {
                id: Uuid::new_v4(),
                name: code.token_name.clone(),
                r#type: TokenType::from(TokenType::API_TOKEN),
                token_hash: hash,
                expires_at: expires.clone(),
                create_time: time.clone(),
                update_time: time,
                ..Default::default()
            })
            .await
            .api("token")?;
        Ok(Response::new(pb::ExchangeSetupCodeResponse {
            token,
            expires_at: Some(conv::convert_time_to_timestamp(&expires)),
            name: code.token_name,
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn durations_use_go_nanosecond_saturation_and_defaults() {
        let parse = |value| {
            duration(
                value,
                DEFAULT_TOKEN_EXPIRY,
                MAX_TOKEN_EXPIRY,
                "expires_in",
                "8760h0m0s",
            )
        };
        assert_eq!(parse(None).unwrap(), DEFAULT_TOKEN_EXPIRY);
        assert_eq!(
            parse(Some(pbjson_types::Duration {
                seconds: 0,
                nanos: 0
            }))
            .unwrap(),
            chrono::Duration::zero()
        );
        assert_eq!(
            parse(Some(pbjson_types::Duration {
                seconds: i64::MIN,
                nanos: 0
            }))
            .unwrap(),
            chrono::Duration::nanoseconds(i64::MIN)
        );
        assert_eq!(
            parse(Some(pbjson_types::Duration {
                seconds: i64::MAX,
                nanos: 0
            }))
            .unwrap_err()
            .code,
            Code::InvalidArgument
        );
        assert_eq!(
            parse(Some(pbjson_types::Duration {
                seconds: 0,
                nanos: -1
            }))
            .unwrap(),
            chrono::Duration::nanoseconds(-1)
        );
    }
}
