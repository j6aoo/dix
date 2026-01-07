use anchor_lang::prelude::*;

declare_id!("DIXxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx");

#[program]
pub mod dix_registry {
    use super::*;

    pub fn register(ctx: Context<Register>, username: String) -> Result<()> {
        require!(username.len() >= 3 && username.len() <= 20, DixError::InvalidUsername);

        let alias = &mut ctx.accounts.alias;
        alias.username = username;
        alias.owner = ctx.accounts.user.key();
        alias.created_at = Clock::get()?.unix_timestamp;

        msg!("Registered: {} -> {}", alias.username, alias.owner);

        Ok(())
    }

    pub fn update(ctx: Context<Update>, new_owner: Pubkey) -> Result<()> {
        let alias = &mut ctx.accounts.alias;
        require!(alias.owner == ctx.accounts.user.key(), DixError::NotOwner);

        let old_owner = alias.owner;
        alias.owner = new_owner;

        msg!("Updated: {} from {} to {}", alias.username, old_owner, new_owner);

        Ok(())
    }
}

#[derive(Accounts)]
#[instruction(username: String)]
pub struct Register<'info> {
    #[account(
        init,
        payer = user,
        space = 8 + Alias::INIT_SPACE,
        seeds = [b"alias", username.as_bytes()],
        bump
    )]
    pub alias: Account<'info, Alias>,

    #[account(mut)]
    pub user: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct Update<'info> {
    #[account(mut)]
    pub alias: Account<'info, Alias>,

    pub user: Signer<'info>,
}

#[account]
#[derive(InitSpace)]
pub struct Alias {
    #[max_len(20)]
    pub username: String,
    pub owner: Pubkey,
    pub created_at: i64,
}

#[error_code]
pub enum DixError {
    #[msg("Username must be 3-20 characters")]
    InvalidUsername,
    #[msg("Not the owner of this alias")]
    NotOwner,
}
